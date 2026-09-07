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

	// inlineAttemptGraceWindow is a fixed, short pause spawnForwardLegs' own
	// goroutine takes before its first leg-2 attempt — every other consumer
	// of this transfer_id (a test-only fault/crash registration arriving a
	// few milliseconds behind the HTTP response that carried this
	// transfer_id back) needs the row to still be unclaimed when it lands.
	// Harmless in production: the synchronous response has already been
	// written by the time this goroutine even starts, so the extra latency
	// is invisible at the driving port (brief.md § Sync vs. async
	// settlement) and only ever shortens the window a stalled leg spends
	// unclaimed.
	inlineAttemptGraceWindow = 20 * time.Millisecond
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
}

// testOnlyFaultState is a per-coordinator (not global) registry, so two
// World instances in the same test binary never share fault state across
// scenarios (each scenario's own httptest.Server wires a fresh
// TransferCoordinator via NewRouter).
type testOnlyFaultState struct {
	mu                sync.Mutex
	legFaults         map[string]bool // key: transferID + ":" + leg, one-shot
	skipForwardCrash  map[string]bool // key: transferID
	skipReversalCrash map[string]bool // key: transferID — recorded for 03-02/04's own reversal path to consult
}

// NewTransferCoordinator wires the saga over an already-constructed Ledger.
func NewTransferCoordinator(ledger *Ledger) *TransferCoordinator {
	return &TransferCoordinator{
		ledger: ledger,
		testOnly: testOnlyFaultState{
			legFaults:         map[string]bool{},
			skipForwardCrash:  map[string]bool{},
			skipReversalCrash: map[string]bool{},
		},
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
// following attempt (inline retry or ticker) runs normally.
func (tc *TransferCoordinator) InjectLegFault(transferID string, leg int) {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.legFaults[legFaultKey(transferID, leg)] = true
}

// consumeInjectedLegFault reports whether a fault was armed for this
// transfer's leg, clearing it on the way out — a one-shot fault only ever
// stalls the first attempt it meets, never every attempt after it.
func (tc *TransferCoordinator) consumeInjectedLegFault(transferID string, leg int) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	key := legFaultKey(transferID, leg)
	if tc.testOnly.legFaults[key] {
		delete(tc.testOnly.legFaults, key)
		return true
	}
	return false
}

func legFaultKey(transferID string, leg int) string {
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
// beyond the sum of its two attempts.
func (tc *TransferCoordinator) spawnForwardLegs(transferID string) {
	go func() {
		// inlineAttemptGraceWindow: see its own doc comment above — gives a
		// test-only fault/crash registration racing this goroutine (it only
		// learns transferID once SendTransfer's HTTP response has already
		// been written) time to land before the first real attempt runs.
		time.Sleep(inlineAttemptGraceWindow)

		if tc.consumeForwardCrashSimulation(transferID) {
			// Test-only: simulating a process crash before Leg 2's first
			// attempt ever ran. The row is left exactly as SendTransfer's
			// own commit left it — only processDueTransfers can claim it
			// from here.
			return
		}

		background := context.Background()
		// A non-nil error here (including a lost claim race, which
		// attemptLeg reports as a nil error/no-op, not this branch) means
		// Leg 2 did not post -- Leg 3 must not be attempted out of order.
		// The retry ticker (step 03-02) is what resumes from here; this
		// goroutine's own job is done once its first attempt has run.
		if err := tc.attemptLeg(background, transferID, 2); err != nil {
			return
		}
		_ = tc.attemptLeg(background, transferID, 3)
	}()
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
func (tc *TransferCoordinator) attemptLeg(ctx context.Context, transferID string, leg int) error {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	state, claimed, err := tc.claimTransfer(attemptCtx, transferID)
	if err != nil || !claimed {
		return err
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
	return tc.recordLegOutcome(attemptCtx, transferID, leg, postErr)
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

// recordLegOutcome is attemptLeg's own "(3) a second write" step (brief.md
// § Retry and reversal mechanics): on success, advancing the transfer to
// settled once Leg 3 posts. Retry/backoff/reversal on failure is slice
// 03/04's own scope, not this step's — a failed attempt today simply
// returns the error, leaving the claim's own 15-second lease as the retry
// cadence until that machinery lands.
func (tc *TransferCoordinator) recordLegOutcome(ctx context.Context, transferID string, leg int, postErr error) error {
	if postErr != nil {
		return postErr
	}

	status := legPending
	if leg == 3 {
		status = "settled"
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
// Backoff scheduling and reversal dispatch on repeated failure are step
// 03-02/03-03's own scope (brief.md § Retry and reversal mechanics) — this
// entrypoint's job today is discovery plus one attempt per discovered row,
// which is exactly what the fault-injection seam (this step) needs to
// single-step deterministically.
func (tc *TransferCoordinator) processDueTransfers(ctx context.Context, batchLimit int) (int, error) {
	dueIDs, err := tc.dueTransferIDs(ctx, batchLimit)
	if err != nil {
		return 0, err
	}
	for _, transferID := range dueIDs {
		leg, err := tc.nextLegFor(ctx, transferID)
		if err != nil || leg == 0 {
			continue
		}
		_ = tc.attemptLeg(ctx, transferID, leg)
	}
	return len(dueIDs), nil
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
