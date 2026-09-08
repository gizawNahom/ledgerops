// transfer_coordinator.go is the per-transfer saga (ADR-015, ADR-015
// Amendment/Amendment 2): SendTransfer posts Leg 1 synchronously and hands
// Legs 2/3 to attemptLeg, the one function both the inline goroutine (this
// step) and the retry ticker (step 03-02) call. TransferCoordinator is
// deliberately its own type, not a method on Ledger (brief.md § Inter-tenant
// transfer, "Component decomposition"): it owns goroutine lifecycle no other
// use case needs, and folding it into Ledger would give every other use
// case an unused dependency on a scheduler it never touches.
package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

const (
	// platformTenantID is the reserved internal tenant every cross-tenant
	// transfer's Leg 2 posts under (migration 0004's own seed row) — never
	// either business tenant's raw tenant_id, keeping every leg strictly
	// intra-tenant per I8.
	platformTenantID = "tnt_platform"

	// settlementAccountName is the one System-kind account each business
	// tenant owns for its own Leg 1/Leg 3 side of a cross-tenant transfer
	// (brief.md § Account bootstrap) — distinct from any wallet a tenant may
	// have opened for itself.
	settlementAccountName = "settlement"

	// attemptTimeout bounds one attemptLeg call's own Post attempt — a
	// per-attempt timeout, not a per-transfer one (brief.md § Sync vs. async
	// settlement): a co-located Go process's expected round trip is
	// milliseconds, so 10s absorbs a slow cold start without masking a
	// genuinely stuck call past the point it stops being useful information.
	attemptTimeout = 10 * time.Second

	// claimLeaseDuration is the transient hold ClaimOne places on a row while
	// an attempt is in flight (brief.md § Crash recovery) — always
	// overwritten by attemptLeg's own outcome-recording write in every
	// non-crash path; only observed at rest when the process dies mid-attempt.
	claimLeaseDuration = 15 * time.Second

	// retryBatchLimit is processDueTransfers' own batch cap (roadmap.json §
	// 03-01/03-02, ClaimDue(ctx, now, batchLimit=100)) — sized against the
	// same discovery query the inline goroutine's own claim shares no
	// exclusivity with (brief.md § ClaimDue's own batch limit).
	retryBatchLimit = 100

	// retryBudget is the fixed number of attempts (US-3's own domain
	// example, wave-decisions.md § Key Decisions: "Fixed retry budget N = 5")
	// a single leg gets before this step's own scope stops rescheduling it —
	// tracked in-memory per coordinator instance (recordFailedAttempt below):
	// sufficient for this step's own scenarios (none of which survive a
	// process crash mid-budget), leaving budget durability across a crash as
	// slice 04's own concern alongside the reversal trigger it builds.
	retryBudget = 5

	// backoffJitterSpread is US-3's own "~20%" jitter band (wave-
	// decisions.md), applied symmetrically around each base delay so the
	// realized delay lands in [0.8x, 1.2x) of the scheduled base.
	backoffJitterSpread = 0.2

	// statusRetrying and statusSettled are transfer_state's own caller-
	// facing status labels (brief.md § Retry and reversal mechanics /
	// D10) — named constants here so every write site agrees on the exact
	// wire string, mirroring legPending/legPosted one section below.
	statusRetrying = "retrying"
	statusSettled  = "settled"

	// inlineAttemptGraceWindow is a fixed, short pause spawnForwardLegs' own
	// goroutine takes before its first leg-2 attempt — every other consumer
	// of this transfer_id (a test-only fault/crash registration arriving a
	// few milliseconds behind the HTTP response that carried this
	// transfer_id back) needs the row to still be unclaimed when it lands.
	//
	// 50ms, not 200us (step 03-03 fix): a second real HTTP round trip
	// (client -> httptest.Server -> handler -> back) routinely costs
	// low-single-digit milliseconds in this suite's own request logs, so a
	// sub-millisecond window let the goroutine's first attempt win the race
	// against the crash-simulation registration nearly every time,
	// defeating "a crash before any Leg 2 attempt is recovered by the retry
	// ticker alone" before the ticker ever got a chance to claim the row.
	// 50ms is comfortable headroom (an order of magnitude above observed
	// same-machine loopback latency) while staying conservative: it does not
	// meaningfully change the product's own latency story, since Legs 2/3
	// are already allowed to complete asynchronously at any point after the
	// synchronous `pending` response (brief.md § Sync vs. async settlement),
	// and 50ms is invisible relative to the existing 10s per-attempt timeout
	// and 1s+ backoff delays. Harmless in production: the synchronous
	// response has already been written by the time this goroutine even
	// starts, so the extra latency is invisible at the driving port and only
	// ever shortens the window a stalled leg spends unclaimed.
	inlineAttemptGraceWindow = 50 * time.Millisecond
)

// ErrTransferNotFound marks a GetTransfer call naming a transfer_id no
// SendTransfer ever created. Deliberately NOT a domain.ViolationKind member
// (brief.md § Inter-tenant transfer, "No new sealed domain.ViolationKind
// member" / ADR-014): Transfer is not a domain aggregate, so there is no
// sealed taxonomy for this fact to join — the HTTP adapter maps this
// sentinel onto 404 transfer_not_found directly, the same way it already
// treats app.ErrIdempotencyKeyConflict as a shell-level, not domain-level,
// refusal.
var ErrTransferNotFound = errors.New("transfer not found")

// TransferCoordinator is the application-layer saga over a cross-tenant
// transfer. It holds no state of its own beyond a reference to the existing
// Ledger — every store/clock/id-generator access it needs is Ledger's own
// (same package, DDD-26 reuse), so no dependency is threaded twice.
type TransferCoordinator struct {
	ledger *Ledger

	// testOnly holds every piece of state the acceptance suite's fault-
	// injection seam needs (step 03-01). Structurally unreachable from any
	// production route: nothing in this file's own SendTransfer/attemptLeg
	// path ever populates it except by calling the InjectLegFault/
	// SimulateCrashBefore*Attempt methods below, and no production
	// composition root (cmd/api/main.go) calls those — only a
	// testonly-build-tagged adapter file does (mirrors
	// postgres.AttemptOutOfBandChange's "back door" precedent: an exported,
	// always-compiled function production code simply never calls).
	testOnly testOnlyFaultState

	// retry is attemptLeg's own in-memory failed-attempt counter (step
	// 03-02) — see retryBudget's own doc comment for why in-memory scope is
	// sufficient here.
	retry retryAttemptTracker
}

// retryAttemptTracker counts consecutive failed attempts per (transfer_id,
// leg) pair — a plain mutex-guarded map, not sync.Map, because every access
// here is a read-increment-write, which sync.Map has no atomic primitive for
// anyway.
type retryAttemptTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

// recordFailedAttempt increments and returns the new failed-attempt count
// for transferID's named leg — the 1-based count backoffForAttempt reads to
// pick the next delay off the schedule.
func (tc *TransferCoordinator) recordFailedAttempt(transferID string, leg int) int {
	tc.retry.mu.Lock()
	defer tc.retry.mu.Unlock()
	key := transferLegKey(transferID, leg)
	tc.retry.counts[key]++
	return tc.retry.counts[key]
}

// testOnlyFaultState is a per-coordinator (not global) registry, so two
// World instances in the same test binary never share fault state across
// scenarios (each scenario's own httptest.Server wires a fresh
// TransferCoordinator via NewRouter).
type testOnlyFaultState struct {
	mu        sync.Mutex
	legFaults map[string]int // key: transferID + ":" + leg, remaining fail count
	// (2026-09-08, DELIVER 03-04 back-propagation): widened from
	// map[string]bool to map[string]int so InjectLegFaultCount can arm a
	// SPECIFIC number of consecutive failures ("fails on its first two
	// attempts"), not just a single one-shot fault -- InjectLegFault's own
	// one-shot behavior is unchanged, expressed as count=1 below.
	skipForwardCrash  map[string]bool // key: transferID
	skipReversalCrash map[string]bool // key: transferID — recorded for 03-02/04's own reversal path to consult
}

// NewTransferCoordinator wires the saga over an already-constructed Ledger.
func NewTransferCoordinator(ledger *Ledger) *TransferCoordinator {
	return &TransferCoordinator{
		ledger: ledger,
		testOnly: testOnlyFaultState{
			legFaults:         map[string]int{},
			skipForwardCrash:  map[string]bool{},
			skipReversalCrash: map[string]bool{},
		},
		retry: retryAttemptTracker{counts: map[string]int{}},
	}
}

// --- test-only fault-injection seam (step 03-01) ---------------------------
//
// Every method below exists for exactly one reason: the acceptance suite
// needs a named back door to force a specific leg to fail, or to simulate a
// process crash at one of the two crash windows, or to seed rows a ticker
// tick can discover — deterministically, without real network faults or
// real wall-clock waiting. None of these methods is reachable from any
// production driving port; only tests/acceptance/intertenanttransfer/world.go,
// through a testonly-build-tagged HTTP adapter
// (internal/adapters/http/testonly_faults.go), ever calls them.

// InjectLegFault forces the next attemptLeg call for the named transfer's
// leg to fail with a simulated transient fault, consumed on first use — the
// following attempt (inline retry or ticker) runs normally. Equivalent to
// InjectLegFaultCount(transferID, leg, 1).
func (tc *TransferCoordinator) InjectLegFault(transferID string, leg int) {
	tc.InjectLegFaultCount(transferID, leg, 1)
}

// InjectLegFaultCount forces the next `count` consecutive attemptLeg calls
// for the named transfer's leg to each fail with a simulated transient
// fault (2026-09-08, DELIVER 03-04 back-propagation) — added for "fails on
// its first two attempts," which InjectLegFault's fixed one-shot semantics
// could not express. count <= 0 arms nothing (a no-op), mirroring
// InjectLegFault's own always-arm-exactly-one contract by construction for
// count == 1.
func (tc *TransferCoordinator) InjectLegFaultCount(transferID string, leg, count int) {
	if count <= 0 {
		return
	}
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.legFaults[transferLegKey(transferID, leg)] = count
}

// consumeInjectedLegFault reports whether a fault was armed for this
// transfer's leg, decrementing the remaining count and clearing the entry
// once exhausted — a fault only ever stalls the next `count` attempts it
// meets, never every attempt after it.
func (tc *TransferCoordinator) consumeInjectedLegFault(transferID string, leg int) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	key := transferLegKey(transferID, leg)
	remaining := tc.testOnly.legFaults[key]
	if remaining <= 0 {
		return false
	}
	remaining--
	if remaining <= 0 {
		delete(tc.testOnly.legFaults, key)
	} else {
		tc.testOnly.legFaults[key] = remaining
	}
	return true
}

// transferLegKey is the shared (transferID, leg) map key format for both
// the test-only fault registry above and attemptLeg's own failed-attempt
// counter below -- one composite key shape, not two independently
// maintained ones.
func transferLegKey(transferID string, leg int) string {
	return fmt.Sprintf("%s:%d", transferID, leg)
}

// errSimulatedTransientFault is the injected failure attemptLeg's own
// recordLegOutcome sees — indistinguishable, from that call's own
// perspective, from a genuine transient Post failure.
var errSimulatedTransientFault = errors.New("simulated transient fault (test-only fault injection)")

// SimulateCrashBeforeForwardLegAttempt marks a transfer so spawnForwardLegs'
// own goroutine returns immediately without ever attempting leg 2 —
// simulating a process crash between Leg 1's commit and the inline
// goroutine's first Leg 2 attempt. The retry ticker (processDueTransfers)
// remains the only path that can ever claim the row afterward.
func (tc *TransferCoordinator) SimulateCrashBeforeForwardLegAttempt(transferID string) {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.skipForwardCrash[transferID] = true
}

// consumeForwardCrashSimulation reports (and clears) whether this transfer
// was marked to skip its inline forward attempt entirely.
func (tc *TransferCoordinator) consumeForwardCrashSimulation(transferID string) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	if tc.testOnly.skipForwardCrash[transferID] {
		delete(tc.testOnly.skipForwardCrash, transferID)
		return true
	}
	return false
}

// SimulateCrashBeforeReversalAttempt marks a transfer so the reversal path
// (step 03-02/04's own scope — no reversal dispatch exists yet) will skip
// attempting an earlier leg's reversal on whatever inline path eventually
// drives it, mirroring SimulateCrashBeforeForwardLegAttempt on the
// compensating side. Recorded now so the seam's shape is stable before the
// reversal dispatcher that must consult it exists; consuming it is that
// dispatcher's own responsibility, not this step's.
func (tc *TransferCoordinator) SimulateCrashBeforeReversalAttempt(transferID string) {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.skipReversalCrash[transferID] = true
}

// ProcessDueTransfersOnce single-steps the ticker's own scan-and-dispatch
// entrypoint (processDueTransfers) exactly once — the exported seam
// RunRetryTickerOnce's HTTP adapter calls, so the acceptance suite never
// needs a real wall-clock sleep to observe one tick's effect.
func (tc *TransferCoordinator) ProcessDueTransfersOnce(ctx context.Context) (int, error) {
	return tc.processDueTransfers(ctx, retryBatchLimit)
}

// SeedTransferStateDueNow persists one synthetic, already-due transfer_state
// row directly through the store — the batch-limit scenario's own seeding
// need (more due rows than one tick's batch cap), which has nothing to do
// with posting real legs and so has no reason to go through SendTransfer's
// full alias-resolution/account-bootstrap pipeline.
func (tc *TransferCoordinator) SeedTransferStateDueNow(ctx context.Context, state ports.TransferState) error {
	return tc.createTransferState(ctx, state)
}

// TransferView is what SendTransfer and GetTransfer both answer with — the
// coordinator's own read shape, never a domain.Transfer (ADR-014: Transfer
// is not a domain aggregate). SendTransfer only ever populates Leg1 (the
// sync/async contract: "pending" with only leg1 posted, never settled) —
// GetTransfer is the one caller that populates all three.
type TransferView struct {
	TransferID string
	Status     string
	Reason     string
	Leg1       LegView
	Leg2       LegView
	Leg3       LegView
}

// LegView is one leg's caller-facing status.
type LegView struct {
	Status string
}

const (
	legPending = "pending"
	legPosted  = "posted"
)

// SendTransfer is the Read → Decide → Write sandwich over three separate
// units of work, one per stage, because Leg 1 goes through the existing,
// unmodified PostTransfer path (which manages its own transaction) rather
// than being inlined here:
//
//  1. Resolve the counterparty alias and bootstrap every settlement/platform-
//     mirror account first-use, idempotently (brief.md § Account bootstrap) —
//     one unit of work, read-then-decide-then-write, mirroring every other
//     courtesy check this codebase already has (DDD-26).
//  2. Post Leg 1 — sender's own wallet to sender's own settlement account —
//     via Ledger.PostTransfer, unmodified, using the caller's own
//     Idempotency-Key exactly as the existing single-tenant path already
//     does. Zero new code for this leg's own posting rule.
//  3. Record the transfer_state row (status pending, leg1 already posted)
//     immediately after Leg 1 commits.
//  4. Spawn Legs 2/3's first attempt in a detached goroutine and return the
//     pending, leg1-only response — the response never blocks for Legs 2/3
//     and never itself reports settled (brief.md § Sync vs. async
//     settlement).
func (tc *TransferCoordinator) SendTransfer(ctx context.Context, tenantID, alias string, amount domain.Money, idempotencyKey, fingerprint string) (TransferView, error) {
	if existing, found, err := tc.existingTransfer(ctx, tenantID, idempotencyKey); err != nil {
		return TransferView{}, err
	} else if found {
		return TransferView{
			TransferID: existing.TransferID,
			Status:     existing.Status,
			Leg1:       LegView{Status: legPosted},
		}, nil
	}

	counterparty, err := tc.prepareTransfer(ctx, tenantID, alias)
	if err != nil {
		return TransferView{}, err
	}

	senderWalletID, err := tc.walletAccountID(ctx, tenantID)
	if err != nil {
		return TransferView{}, err
	}

	if _, err := tc.ledger.PostTransfer(ctx, TransferRequest{
		From:           senderWalletID,
		To:             settlementAccountName,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
		Fingerprint:    fingerprint,
		TenantID:       tenantID,
	}); err != nil {
		return TransferView{}, err
	}

	transferID := "xfr_" + tc.ledger.nextID()
	state := ports.TransferState{
		TransferID:           transferID,
		TenantID:             tenantID,
		IdempotencyKey:       idempotencyKey,
		Status:               legPending,
		Leg1Status:           legPosted,
		Leg2Status:           legPending,
		Leg3Status:           legPending,
		NextAttemptAt:        tc.ledger.clock(),
		CounterpartyTenantID: counterparty.tenantID,
		TargetAccountID:      counterparty.accountID,
		Amount:               amount,
	}
	if err := tc.createTransferState(ctx, state); err != nil {
		return TransferView{}, err
	}

	tc.spawnForwardLegs(transferID)

	return TransferView{
		TransferID: transferID,
		Status:     legPending,
		Leg1:       LegView{Status: legPosted},
	}, nil
}

// existingTransfer is SendTransfer's transfer-level replay check (step
// 02-06, migration 02-01's UNIQUE(tenant_id, idempotency_key) constraint) —
// a DISTINCT mechanism from the per-leg IdempotencyStore replay Leg 1's own
// PostTransfer call already performs (brief.md § For Acceptance Designer,
// "dual idempotency mechanisms"). Read-before-write: a resend of an
// identical (tenant_id, idempotency_key) pair must answer with the ALREADY
// recorded transfer, never re-run alias resolution or post any leg again.
// Scoped by BOTH tenant_id AND idempotency_key together, so two distinct
// keys from the same tenant to the same alias produce two independent
// transfers.
func (tc *TransferCoordinator) existingTransfer(ctx context.Context, tenantID, idempotencyKey string) (ports.TransferState, bool, error) {
	result, err := withUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("checking for an existing transfer under tenant %q idempotency key", tenantID),
		func(uow ports.UnitOfWork) (existingTransferLookup, error) {
			state, found, err := uow.TransferStates().ByTenantAndIdempotencyKey(ctx, tenantID, idempotencyKey)
			return existingTransferLookup{state: state, found: found}, err
		})
	return result.state, result.found, err
}

// existingTransferLookup is existingTransfer's own return shape — bundling
// the (value, found) pair withUnitOfWork's single-value generic signature
// cannot carry directly, mirroring resolvedCounterparty one call site below.
type existingTransferLookup struct {
	state ports.TransferState
	found bool
}

// resolvedCounterparty is prepareTransfer's own return shape — the two
// facts SendTransfer needs from alias resolution besides the bootstrap
// side-effects already applied.
type resolvedCounterparty struct {
	tenantID  string
	accountID string
}

// prepareTransfer resolves the named alias and bootstraps every account a
// cross-tenant transfer touches before Leg 1's own account lock ever runs
// (brief.md § Account bootstrap: "idempotent get-or-create, inside
// SendTransfer, first-use"). One unit of work: the alias/link reads that
// decide ResolveCounterparty, then the four idempotent account-bootstrap
// writes, all-or-nothing together.
func (tc *TransferCoordinator) prepareTransfer(ctx context.Context, tenantID, alias string) (resolvedCounterparty, error) {
	return withUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("preparing cross-tenant transfer for tenant %q alias %q", tenantID, alias),
		func(uow ports.UnitOfWork) (resolvedCounterparty, error) {
			aliasSnapshot, aliasFound, err := uow.CounterpartyAliases().ByTenantAndAlias(ctx, tenantID, alias)
			if err != nil {
				return resolvedCounterparty{}, fmt.Errorf("resolving counterparty alias %q for tenant %q: %w", alias, tenantID, err)
			}

			linkSnapshot, linkFound, err := tc.linkSnapshotFor(ctx, uow, aliasSnapshot, aliasFound)
			if err != nil {
				return resolvedCounterparty{}, err
			}

			counterpartyTenantID, targetAccountID, err := domain.ResolveCounterparty(
				tenantID, alias, aliasSnapshot, aliasFound, linkSnapshot, linkFound)
			if err != nil {
				return resolvedCounterparty{}, err
			}

			if err := tc.ensureSettlementAccount(ctx, uow, tenantID); err != nil {
				return resolvedCounterparty{}, err
			}
			if err := tc.ensurePlatformMirrorAccount(ctx, uow, tenantID); err != nil {
				return resolvedCounterparty{}, err
			}
			if err := tc.ensureSettlementAccount(ctx, uow, counterpartyTenantID); err != nil {
				return resolvedCounterparty{}, err
			}
			if err := tc.ensurePlatformMirrorAccount(ctx, uow, counterpartyTenantID); err != nil {
				return resolvedCounterparty{}, err
			}

			return resolvedCounterparty{tenantID: counterpartyTenantID, accountID: targetAccountID}, nil
		})
}

// linkSnapshotFor performs the impure read behind ResolveCounterparty's own
// link-still-active check — mirroring RegisterCounterpartyAlias's identical
// courtesy-check shape (usecases.go) one call site over. Only read when an
// alias was actually found: an absent alias needs no link lookup at all.
func (tc *TransferCoordinator) linkSnapshotFor(ctx context.Context, uow ports.UnitOfWork, aliasSnapshot domain.CounterpartyAlias, aliasFound bool) (domain.TenantLink, bool, error) {
	if !aliasFound {
		return domain.TenantLink{}, false, nil
	}
	link, err := uow.TenantLinks().ByID(ctx, aliasSnapshot.TenantLinkID())
	if err == nil {
		return link, true, nil
	}
	var violation domain.Violation
	if errors.As(err, &violation) && violation.Kind() == domain.TenantLinkNotFound {
		return domain.TenantLink{}, false, nil
	}
	return domain.TenantLink{}, false, fmt.Errorf("reading tenant link %q for alias resolution: %w", aliasSnapshot.TenantLinkID(), err)
}

// walletAccountID finds the sending tenant's own wallet account — the wire
// request names only a counterparty_alias and an amount, never a from
// account (brief.md's own illustrative request shape carries none), so the
// sender's wallet is discovered by convention: the one Wallet-kind account
// this tenant has opened. Refuses account_not_found, naming "wallet", if
// none exists yet.
func (tc *TransferCoordinator) walletAccountID(ctx context.Context, tenantID string) (string, error) {
	return withUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("locating tenant %q's wallet account", tenantID),
		func(uow ports.UnitOfWork) (string, error) {
			accounts, err := uow.Accounts().All(ctx, tenantID)
			if err != nil {
				return "", fmt.Errorf("locating tenant %q's wallet account: %w", tenantID, err)
			}
			for _, account := range accounts {
				if account.Kind() == domain.Wallet {
					return account.ID(), nil
				}
			}
			return "", domain.NewUnknownAccount("wallet")
		})
}

// ensureSettlementAccount and ensurePlatformMirrorAccount are the two named
// account-bootstrap shapes brief.md's own leg-to-tenant mapping requires —
// factored apart so each call site at prepareTransfer reads as one domain
// sentence rather than three positional arguments repeated four times.
func (tc *TransferCoordinator) ensureSettlementAccount(ctx context.Context, uow ports.UnitOfWork, tenantID string) error {
	return tc.ensureAccount(ctx, uow, tenantID, settlementAccountName)
}

func (tc *TransferCoordinator) ensurePlatformMirrorAccount(ctx context.Context, uow ports.UnitOfWork, businessTenantID string) error {
	return tc.ensureAccount(ctx, uow, platformTenantID, platformMirrorAccountID(businessTenantID))
}

// platformMirrorAccountID is the one naming convention every leg-2 movement
// shares: tnt_platform's own mirror account for one business tenant's side
// of the ledger (brief.md § Account bootstrap: "platform-{tenant_id}").
func platformMirrorAccountID(businessTenantID string) string {
	return "platform-" + businessTenantID
}

// ensureAccount is the idempotent, first-use get-or-create bootstrap
// (brief.md § Account bootstrap: "this bootstrap is internal and
// deliberately idempotent, so 'already exists' is the expected, successful
// case, not a refusal to swallow") — deliberately calling
// AccountRepository.Get/Create directly rather than reusing OpenAccount's
// alreadyOpen-refuses-hard contract, which exists for a caller-driven
// POST /accounts conflict, a different meaning of "already exists" than
// this internal, repeatable bootstrap. Every bootstrapped account is
// System-kind (D7): I4's wallet floor never applies to a settlement or
// platform-mirror account, only I1 governs it.
func (tc *TransferCoordinator) ensureAccount(ctx context.Context, uow ports.UnitOfWork, tenantID, accountID string) error {
	_, err := uow.Accounts().Get(ctx, tenantID, accountID)
	if err == nil {
		return nil
	}
	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.UnknownAccount {
		return fmt.Errorf("checking whether account %q is already open under tenant %q: %w", accountID, tenantID, err)
	}

	opening, err := zeroMoney(ledgerCurrency)
	if err != nil {
		return err
	}
	account, err := domain.NewAccount(tenantID, accountID, domain.System, opening)
	if err != nil {
		return err
	}
	if err := uow.Accounts().Create(ctx, tenantID, account); err != nil {
		return fmt.Errorf("bootstrapping account %q under tenant %q: %w", accountID, tenantID, err)
	}
	return nil
}

// createTransferState persists the coordinator's own progress row, in its
// own unit of work, immediately after Leg 1's commit (brief.md § Coordinator
// state persistence: "committed before the handler returns").
func (tc *TransferCoordinator) createTransferState(ctx context.Context, state ports.TransferState) error {
	return inUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("recording transfer state %q", state.TransferID),
		func(uow ports.UnitOfWork) error {
			return uow.TransferStates().Create(ctx, state)
		})
}

// spawnForwardLegs attempts Legs 2 then 3 in a goroutine detached from the
// request's own context (brief.md § Sync vs. async settlement: "given its
// own bounded context.Background() + timeout") — "inline" means "attempted
// at once, without waiting for the ticker," not "before the HTTP response
// is written." Each attemptLeg call below gets its own fresh per-attempt
// timeout internally; this goroutine itself carries no umbrella timeout
// beyond the sum of its own attempts (including any self-rescheduled
// retries — see attemptForwardLegsFrom).
func (tc *TransferCoordinator) spawnForwardLegs(transferID string) {
	go func() {
		// inlineAttemptGraceWindow: see its own doc comment above — gives a
		// test-only fault/crash registration racing this goroutine (it only
		// learns transferID once SendTransfer's HTTP response has already
		// been written) time to land before the first real attempt runs.
		time.Sleep(inlineAttemptGraceWindow)

		tc.attemptForwardLegsFrom(context.Background(), transferID, 2)
	}()
}

// attemptForwardLegsFrom attempts one leg, and — if it succeeds and it was
// Leg 2 — immediately continues to Leg 3, exactly as the original inline
// sequence always has (brief.md § Sync vs. async settlement). This is the
// one place that sequencing rule lives, shared by spawnForwardLegs' first
// attempt above and every self-rescheduled retry a failure spawns
// (scheduleRetry below) — a successful retry completes the sequence
// identically to a successful first attempt, never leaving Leg 3
// unattempted just because Leg 2 needed a retry to get there. A failed
// attempt returns without continuing: handleLegFailure (inside attemptLeg)
// has already scheduled its own retry, and Leg 3 must never be attempted
// before Leg 2 has actually posted.
func (tc *TransferCoordinator) attemptForwardLegsFrom(ctx context.Context, transferID string, leg int) {
	if err := tc.attemptLeg(ctx, transferID, leg, true); err != nil || leg != 2 {
		return
	}
	_ = tc.attemptLeg(ctx, transferID, 3, true)
}

// attemptLeg is the one function both the inline goroutine above and the
// retry ticker (step 03-02) call — self-claims via ClaimOne before running
// anything, so a losing claim race (or a not-yet-due row) is a normal,
// silent no-op, never an error (brief.md § Crash recovery). Every fact this
// function needs — which accounts move, how much, under which tenant, under
// which synthesized idempotency key — comes from the persisted
// transfer_state row alone, never from caller-supplied context, which is
// what makes it equally correct whether invoked moments after SendTransfer
// or minutes later by a ticker that only just discovered the row.
//
// isInlinePath distinguishes attemptForwardLegsFrom's own callers (the
// inline goroutine spawnForwardLegs starts, plus its self-rescheduled
// retries) from processDueTransfers' ticker path (step 03-03 fix): only the
// inline path may ever have been told, via
// SimulateCrashBeforeForwardLegAttempt, to simulate a crash before its own
// first Leg 2 attempt. Before this parameter existed, the crash-simulation
// check below fired for WHICHEVER caller's attemptLeg call claimed the row
// first — including the ticker's own attemptDueLegs call, which, once
// inlineAttemptGraceWindow was widened enough to let the ticker reliably
// win that claim race, started wrongly self-releasing on the ticker's own
// dispatch instead of ever posting Leg 2 for real, permanently starving "a
// crash before any Leg 2 attempt is recovered by the retry ticker alone" of
// the one caller meant to recover it.
//
// ctx (not attemptCtx) is deliberately what handleLegFailure/scheduleRetry
// receive below: attemptCtx's own timeout/cancellation is scoped to this one
// Post attempt (attemptTimeout) and must not leak into a retry goroutine
// that may still be sleeping long after this call returns.
func (tc *TransferCoordinator) attemptLeg(ctx context.Context, transferID string, leg int, isInlinePath bool) error {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	state, claimed, err := tc.claimTransfer(attemptCtx, transferID)
	if err != nil || !claimed {
		return err
	}

	if leg == 2 && isInlinePath && tc.consumeForwardCrashSimulation(transferID) {
		// Test-only: simulating a process crash between Leg 1's commit and
		// Leg 2's first attempt ever running. Checked HERE — after the
		// claim above, at the same point consumeInjectedLegFault is
		// checked below — rather than as an early gate before this
		// goroutine ever claimed: a test-only registration racing this
		// goroutine only learns transferID once SendTransfer's HTTP
		// response has already been written, so it needs the SAME
		// claim-transaction-latency buffer InjectLegFault's own check
		// already reliably relies on to land in time (an early gate,
		// checked immediately after inlineAttemptGraceWindow with no
		// further buffer, lost that race far more often). The claim above
		// already extended the lease by claimLeaseDuration — release it
		// immediately via advanceAfterLegOutcome rather than leaving the
		// row parked for a full lease's worth of otherwise-unnecessary
		// recovery latency, so the row is due again exactly as if this
		// crashed process had never claimed it at all. isInlinePath is what
		// keeps this branch reachable ONLY from the inline path above — the
		// ticker's own attemptDueLegs call below always passes false, so it
		// never mistakes this test-only flag for its own instruction to
		// stand down, and instead posts Leg 2 for real.
		return tc.advanceAfterLegOutcome(ctx, transferID, state.Status, tc.ledger.clock(), state.Reason)
	}

	from, to, tenantID, key, err := legMovement(state, leg)
	if err != nil {
		return err
	}

	var postErr error
	if tc.consumeInjectedLegFault(transferID, leg) {
		// Test-only: this attempt is armed to fail without ever reaching
		// the ledger — the claim above still ran for real, so this counts
		// as a genuine attempt from the caller/ticker's own perspective.
		postErr = errSimulatedTransientFault
	} else {
		_, postErr = tc.ledger.PostTransfer(attemptCtx, TransferRequest{
			From:           from,
			To:             to,
			Amount:         state.Amount,
			IdempotencyKey: key,
			Fingerprint:    legFingerprint(from, to, state.Amount),
			TenantID:       tenantID,
		})
	}
	if postErr != nil {
		return tc.handleLegFailure(ctx, transferID, leg, postErr)
	}
	return tc.recordLegSuccess(attemptCtx, transferID, leg)
}

// backoffSchedule is the base (pre-jitter) delay before the attempt
// following a failed attempt, indexed by how many attempts have already
// failed (1-based; wave-decisions.md § Key Decisions: "1s/2s/4s/8s", so
// attempts 1->2, 2->3, 3->4, 4->5 carry these four delays). Index 0 is
// never read (backoffForAttempt rejects failedAttempts < 1).
var backoffSchedule = [...]time.Duration{
	1: 1 * time.Second,
	2: 2 * time.Second,
	3: 4 * time.Second,
	4: 8 * time.Second,
}

// backoffForAttempt is the pure function behind the retry schedule (US-3's
// own domain example, wave-decisions.md: fixed budget N=5, backoff
// 1s/2s/4s/8s + ~20% jitter). failedAttempts is how many attempts have
// already failed for one leg (1-based, from recordFailedAttempt). jitter
// must be in [0, 1) — the caller supplies a fresh pseudo-random draw per
// call, kept as an explicit parameter (rather than reading math/rand
// internally) so this function stays a pure, deterministically testable
// unit. Once the budget is exhausted (failedAttempts >= retryBudget), this
// returns (0, false): there is no delay to schedule because there is no
// further attempt this step's own scope reschedules.
func backoffForAttempt(failedAttempts int, jitter float64) (time.Duration, bool) {
	if failedAttempts < 1 || failedAttempts >= retryBudget {
		return 0, false
	}
	base := backoffSchedule[failedAttempts]
	multiplier := (1 - backoffJitterSpread) + jitter*(2*backoffJitterSpread)
	return time.Duration(float64(base) * multiplier), true
}

// handleLegFailure is attemptLeg's own failure path (brief.md § Retry and
// reversal mechanics): records the failed attempt, advances the transfer to
// "retrying" with next_attempt_at set to the computed backoff target, and —
// while the budget is not yet exhausted — self-schedules the retry that
// will pick this same leg back up once it becomes due again. Budget
// exhaustion is left as a narrower scope for this step (see retryBudget's
// own doc comment): the row is left "retrying," still due, for slice 04's
// own reversal trigger to eventually notice and act on.
func (tc *TransferCoordinator) handleLegFailure(ctx context.Context, transferID string, leg int, postErr error) error {
	failedAttempts := tc.recordFailedAttempt(transferID, leg)
	delay, ok := backoffForAttempt(failedAttempts, rand.Float64())

	nextAttemptAt := tc.ledger.clock()
	if ok {
		nextAttemptAt = nextAttemptAt.Add(delay)
	}
	if err := tc.advanceAfterLegOutcome(ctx, transferID, statusRetrying, nextAttemptAt, ""); err != nil {
		return err
	}

	if ok {
		tc.scheduleRetry(transferID, leg, delay)
	}
	return postErr
}

// scheduleRetry is attemptLeg's own self-rescheduling mechanism — a
// detached goroutine that sleeps for exactly the computed backoff delay,
// then re-enters the same claim-gated attemptForwardLegsFrom sequence this
// leg's very first attempt went through. Because a retry re-enters through
// attemptLeg's own ClaimOne, a concurrent ticker-driven claim on the
// identical row (once the ticker exists, step 03-03) can never race this
// goroutine unsafely: exactly one of them wins the claim (brief.md § Crash
// recovery), and the loser's attemptLeg returns immediately without
// attempting anything, exactly like any other lost claim race.
func (tc *TransferCoordinator) scheduleRetry(transferID string, leg int, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		tc.attemptForwardLegsFrom(context.Background(), transferID, leg)
	}()
}

// claimTransfer performs ClaimOne's own atomic, transaction-scoped claim
// (brief.md § Crash recovery) — its own unit of work, committed
// independently of the attempt that follows, exactly as the design's own
// "(1) the atomic claim... (2) the attempt itself... (3) a second write"
// sequencing requires. ok=false is a normal outcome (already claimed, or
// not yet due), never an error.
func (tc *TransferCoordinator) claimTransfer(ctx context.Context, transferID string) (ports.TransferState, bool, error) {
	uow, err := tc.ledger.store.Begin(ctx)
	if err != nil {
		return ports.TransferState{}, false, fmt.Errorf("claiming transfer %q: %w", transferID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	state, ok, err := uow.TransferStates().ClaimOne(ctx, transferID, claimLeaseDuration)
	if err != nil {
		return ports.TransferState{}, false, fmt.Errorf("claiming transfer %q: %w", transferID, err)
	}
	if !ok {
		return ports.TransferState{}, false, nil
	}
	if err := uow.Commit(ctx); err != nil {
		return ports.TransferState{}, false, fmt.Errorf("claiming transfer %q: %w", transferID, err)
	}
	committed = true
	return state, true, nil
}

// legMovement is the pure decision behind which two accounts, under which
// tenant scope, under which synthesized idempotency key, one leg number
// names — brief.md's own leg-to-tenant mapping (§ Account bootstrap),
// restated as code: Leg 2 is entirely within tnt_platform's own tenant
// scope (sender's mirror to receiver's mirror); Leg 3 is entirely within
// the receiving tenant's own scope (its settlement account to its real
// target account). Every leg's From/To share one tenant_id by construction
// here, before Post's own I8 check ever runs a second time.
func legMovement(state ports.TransferState, leg int) (from, to, tenantID, key string, err error) {
	switch leg {
	case 2:
		return platformMirrorAccountID(state.TenantID),
			platformMirrorAccountID(state.CounterpartyTenantID),
			platformTenantID,
			state.IdempotencyKey + ":leg2",
			nil
	case 3:
		return settlementAccountName,
			state.TargetAccountID,
			state.CounterpartyTenantID,
			state.IdempotencyKey + ":leg3",
			nil
	default:
		return "", "", "", "", fmt.Errorf("attemptLeg: unsupported leg number %d", leg)
	}
}

// legFingerprint mirrors the HTTP adapter's own fingerprintTransfer shape
// (internal/adapters/http/handlers.go) one layer down: the idempotency
// fingerprint is computed over the PARSED movement, not raw wire bytes,
// exactly like the caller-facing Leg 1 fingerprint already is.
func legFingerprint(from, to string, amount domain.Money) string {
	return fmt.Sprintf("%s|%s|%d|%s", from, to, amount.MinorUnits(), amount.Currency())
}

// recordLegSuccess is attemptLeg's own "(3) a second write" step on the
// success path (brief.md § Retry and reversal mechanics): advancing the
// transfer to settled once Leg 3 posts, or back to pending (its next leg
// still owed) otherwise. The failure path's own second write is
// handleLegFailure, above.
func (tc *TransferCoordinator) recordLegSuccess(ctx context.Context, transferID string, leg int) error {
	status := legPending
	if leg == 3 {
		status = statusSettled
	}
	// next_attempt_at resets to now on every successful attempt (brief.md §
	// Retry and reversal mechanics, "a second write... sets next_attempt_at
	// down from the lease"): a non-final leg's success must leave the row
	// immediately due again, so the very next leg — attempted moments later
	// by this same goroutine, or later by the ticker after a crash — is
	// never blocked behind ClaimOne's own transient 15-second lease.
	return tc.advanceAfterLegOutcome(ctx, transferID, status, tc.ledger.clock(), "")
}

func (tc *TransferCoordinator) advanceAfterLegOutcome(ctx context.Context, transferID, status string, nextAttemptAt time.Time, reason string) error {
	return inUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("advancing transfer %q to status %q", transferID, status),
		func(uow ports.UnitOfWork) error {
			return uow.TransferStates().AdvanceAfterLegOutcome(ctx, transferID, status, nextAttemptAt, reason)
		})
}

// GetTransfer is the pure-function, return-only driving surface (Core
// Principle 12) — it exposes no write method of its own, mirroring
// TransferStateRepository.Get's own read/write split. Leg 1's status is
// always "posted" once a row exists at all: SendTransfer creates the row
// only after Leg 1's own commit, never before. Legs 2/3's status is derived
// from the SAME synthesized IdempotencyStore keys attemptLeg claims through
// (brief.md: "transfer_state itself carries no independent double-
// application guard... the guard is IdempotencyStore's unique constraint"),
// rather than duplicated into a second, redundant column this function
// would have to keep in sync.
func (tc *TransferCoordinator) GetTransfer(ctx context.Context, transferID string) (TransferView, error) {
	return withUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("reading transfer %q", transferID),
		func(uow ports.UnitOfWork) (TransferView, error) {
			state, found, err := uow.TransferStates().Get(ctx, transferID)
			if err != nil {
				return TransferView{}, fmt.Errorf("reading transfer %q: %w", transferID, err)
			}
			if !found {
				return TransferView{}, ErrTransferNotFound
			}

			leg2Posted, err := legPostedStatus(ctx, uow, state.IdempotencyKey+":leg2")
			if err != nil {
				return TransferView{}, err
			}
			leg3Posted, err := legPostedStatus(ctx, uow, state.IdempotencyKey+":leg3")
			if err != nil {
				return TransferView{}, err
			}

			return TransferView{
				TransferID: state.TransferID,
				Status:     state.Status,
				Reason:     state.Reason,
				Leg1:       LegView{Status: legPosted},
				Leg2:       LegView{Status: leg2Posted},
				Leg3:       LegView{Status: leg3Posted},
			}, nil
		})
}

// processDueTransfers is the ticker's own scan-and-dispatch entrypoint
// (brief.md § Application Architecture): batch-limited discovery via
// ClaimDue, then attemptLeg per discovered row — the same self-claiming
// function the inline goroutine already calls, so a row this call claims
// and a row the inline goroutine claims share identical safety guarantees.
//
// Deliberately calls attemptLeg directly, NOT attemptForwardLegsFrom
// (unlike spawnForwardLegs' own inline path): attemptForwardLegsFrom
// decides whether to continue into leg 3 purely from attemptLeg's return
// value being nil, but nil is ALSO what a losing claim race returns
// (claimTransfer's own "ok=false is a normal outcome, never an error"
// contract) — indistinguishable, from that return value alone, from a
// genuine successful post. Two ticks racing the identical due transfer_id
// (TestAttemptLeg_ConcurrentClaimRace_ExactlyOneCallerAttemptsThePost,
// internal/app/transfer_coordinator_claim_race_test.go) would otherwise let
// the LOSING caller misread its own no-op as "leg 2 posted, continue to leg
// 3", attempting a claim on leg 3 that test never expects. Re-deriving the
// next due leg from persisted state after each attempt (the loop below),
// rather than trusting attemptLeg's own silence, sidesteps that ambiguity
// entirely.
//
// Looping while nextLegFor still names something to do (bounded at two
// iterations — leg 2 then leg 3, never more) is what lets one tick fully
// recover a transfer whose leg 2 never even got a first inline attempt
// (step 03-03, "a crash before any Leg 2 attempt is recovered by the retry
// ticker alone"): next_attempt_at is set at row-creation time (ADR-015
// Amendment), so such a row is immediately due, and the ticker is the only
// path left to advance it once the inline goroutine never ran at all. Once
// this call's own attemptLeg posts leg 2 for real, recordLegSuccess resets
// next_attempt_at to now — nextLegFor's own fresh read then finds leg 3
// next-due in the SAME dispatch, without needing a second, separately-
// triggered tick that would otherwise widen recovery latency beyond the
// documented lease_duration + one ticker interval bound (brief.md § Crash
// recovery). Reversal dispatch on repeated failure remains slice 04's own
// scope.
func (tc *TransferCoordinator) processDueTransfers(ctx context.Context, batchLimit int) (int, error) {
	dueIDs, err := tc.dueTransferIDs(ctx, batchLimit)
	if err != nil {
		return 0, err
	}
	for _, transferID := range dueIDs {
		tc.attemptDueLegs(ctx, transferID)
	}
	return len(dueIDs), nil
}

// attemptDueLegs attempts one discovered transfer's next-due leg, then —
// ONLY if that attempt made real forward progress — the leg that becomes
// next-due immediately after. "Made real forward progress" is judged by
// re-reading nextLegFor rather than trusting attemptLeg's own return value
// (see processDueTransfers' own doc comment for why that value alone is
// ambiguous between "posted" and "lost the claim race"): if the leg named
// after the attempt is UNCHANGED from the leg named before it, nothing this
// caller did advanced the transfer — either the post itself failed, or
// another caller (or this one, if InjectLegFault/crash-forward left nothing
// to claim) already holds or held the claim — so a second attempt is
// skipped rather than risking an extra, unearned ClaimOne call on a
// concurrently-claimed row (TestAttemptLeg_ConcurrentClaimRace_
// ExactlyOneCallerAttemptsThePost, transfer_coordinator_claim_race_test.go,
// asserts exactly one ClaimOne call per racing caller). When the leg DOES
// change (this caller's own attemptLeg really did post it), the newly-due
// leg is attempted too, in the same dispatch — this is what lets one tick
// fully recover a transfer whose leg 2 never even got a first inline
// attempt (step 03-03: leg 2 posts, recordLegSuccess resets next_attempt_at
// to now, and leg 3 is immediately next-due, all within this single call).
func (tc *TransferCoordinator) attemptDueLegs(ctx context.Context, transferID string) {
	leg, err := tc.nextLegFor(ctx, transferID)
	if err != nil || leg == 0 {
		return
	}
	_ = tc.attemptLeg(ctx, transferID, leg, false)

	nextLeg, err := tc.nextLegFor(ctx, transferID)
	if err != nil || nextLeg == 0 || nextLeg == leg {
		return
	}
	_ = tc.attemptLeg(ctx, transferID, nextLeg, false)
}

// dueTransferIDs is processDueTransfers' own read-only discovery step —
// ClaimDue itself claims nothing (brief.md: "discovery only, not a claim"),
// so this runs in its own unit of work, separate from the claim/attempt
// pair attemptLeg performs per discovered row.
func (tc *TransferCoordinator) dueTransferIDs(ctx context.Context, batchLimit int) ([]string, error) {
	return withUnitOfWork(ctx, tc.ledger.store, "discovering due transfers for one retry-ticker tick",
		func(uow ports.UnitOfWork) ([]string, error) {
			return uow.TransferStates().ClaimDue(ctx, tc.ledger.clock(), batchLimit)
		})
}

// nextLegFor reads a due row fresh and decides which leg processDueTransfers
// should attempt next — leg 2 while it has not yet posted, leg 3 once leg 2
// has, and 0 (a no-op sentinel) once both already have, mirroring
// GetTransfer's own leg-status derivation one call site over.
func (tc *TransferCoordinator) nextLegFor(ctx context.Context, transferID string) (int, error) {
	return withUnitOfWork(ctx, tc.ledger.store, fmt.Sprintf("deciding which leg transfer %q is due for", transferID),
		func(uow ports.UnitOfWork) (int, error) {
			state, found, err := uow.TransferStates().Get(ctx, transferID)
			if err != nil || !found {
				return 0, err
			}
			leg2Posted, err := legPostedStatus(ctx, uow, state.IdempotencyKey+":leg2")
			if err != nil {
				return 0, err
			}
			if leg2Posted != legPosted {
				return 2, nil
			}
			leg3Posted, err := legPostedStatus(ctx, uow, state.IdempotencyKey+":leg3")
			if err != nil {
				return 0, err
			}
			if leg3Posted != legPosted {
				return 3, nil
			}
			return 0, nil
		})
}

// legPostedStatus answers one leg's caller-facing status by looking up its
// own synthesized IdempotencyStore key — "posted" on a hit, "pending"
// otherwise (retry/backoff labels beyond that are slice 03's own scope).
func legPostedStatus(ctx context.Context, uow ports.UnitOfWork, key string) (string, error) {
	_, found, err := uow.Idempotency().Lookup(ctx, key)
	if err != nil {
		return "", fmt.Errorf("checking leg posting status for key %q: %w", key, err)
	}
	if found {
		return legPosted, nil
	}
	return legPending, nil
}
