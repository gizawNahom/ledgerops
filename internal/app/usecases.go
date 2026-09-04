// Package app is the effect shell. It sequences read, decide, and write, and
// holds every ordering rule the pure core cannot express: lock acquisition
// order, transaction demarcation, and idempotency-key insertion.
//
// The dependency rule: the shell may call the core, the core never calls the
// shell, and the core does not know the shell exists.
//
// PostTransfer, CreateAccount, and GetBalance are real as of step 01-03: the
// Read → Decide → Write sandwich over the real postgres repositories, one
// database transaction per call. GetEntries is real as of step 02-01 and
// carries the full traceability contract as of milestone-05: running balance
// and unknown-account refusal. VerifyBooks is real as of step 06-01,
// narrowly: the healthy/empty verdict over a full scan (D9); the
// corruption-attribution scenarios (06-02..06-04) are what exercise the
// Drifted rows in anger.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// ledgerCurrency is the ledger's single configured currency. Every account is
// opened in it; a movement across currencies never reaches this shell because
// nothing here ever constructs a second one (see domain.CurrencyMismatch —
// currently unreachable through any driving port, by design).
const ledgerCurrency = "USD"

// legacyTenantID is the sentinel every pre-existing row was backfilled to
// (migration 0003_tenants.up.sql, tnt_legacy_seed). As of step 02-04 the
// write-path use cases (PostTransfer, CreateAccount, GetBalance) no longer
// hardcode it — each now takes the caller's real tenant_id, threaded from
// the HTTP layer's requireTenantKey-resolved scope (router.go/handlers.go).
// VerifyBooks (step 06-01/03-01) still names the sentinel directly for its
// accounts-enumeration leg, but only on the Unscoped() path — that is what
// keeps an unscoped call byte-identical to the pre-multitenancy contract
// (step 03-01's regression requirement). A ScopedToTenant(id) call
// enumerates that tenant's own accounts instead.
const legacyTenantID = "tnt_legacy_seed"

// ErrIdempotencyKeyConflict marks a same-key request whose fingerprint does
// not match the one the key was first claimed with (DDD-8). This is an
// application-shell decision, not a domain.ViolationKind member: no rule in
// the pure core is broken by the request itself, only the shell's
// replay-or-refuse contract over the key. The adapter maps it onto
// idempotency_key_conflict/409 (ADR-008), the same status family as
// account_already_exists — the identifier is already bound to something
// else.
var ErrIdempotencyKeyConflict = errors.New("idempotency key claimed by a different request")

// Ledger is the application layer. Every driving adapter goes through it and
// nothing else.
type Ledger struct {
	store  ports.Store
	clock  ports.Clock
	nextID ports.IDGenerator
}

// NewLedger wires the shell. The two function-typed ports arrive here rather
// than being reached for, which is what makes every use case deterministic
// under test.
func NewLedger(store ports.Store, clock ports.Clock, nextID ports.IDGenerator) *Ledger {
	return &Ledger{store: store, clock: clock, nextID: nextID}
}

// withUnitOfWork runs fn inside one unit of work: opens it, commits on
// success, and rolls back on any error path (including a panic passing
// through fn). CreateAccount, GetBalance, and GetEntries each repeated this
// open/commit-or-rollback ceremony on their own; consolidating it here is
// what stops a future read or write use case repeating it a fourth way.
// describe names the operation for the Begin/Commit error wrap, matching
// what each call site already said before this consolidation.
//
// PostTransfer does NOT use this helper: its sandwich has an early
// commit-and-return on the replay path and a recursive retry on a lost
// idempotency-claim race, neither of which fits this shape without
// obscuring the control flow the ADR-005 commentary there depends on.
func withUnitOfWork[T any](ctx context.Context, store ports.Store, describe string, fn func(ports.UnitOfWork) (T, error)) (T, error) {
	var zero T
	uow, err := store.Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", describe, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	result, err := fn(uow)
	if err != nil {
		return zero, err
	}
	if err := uow.Commit(ctx); err != nil {
		return zero, fmt.Errorf("%s: %w", describe, err)
	}
	committed = true
	return result, nil
}

// inUnitOfWork is withUnitOfWork specialised to actions with no result value
// besides success or failure — CreateAccount is the only call site that
// needs this shape.
func inUnitOfWork(ctx context.Context, store ports.Store, describe string, fn func(ports.UnitOfWork) error) error {
	_, err := withUnitOfWork(ctx, store, describe, func(uow ports.UnitOfWork) (struct{}, error) {
		return struct{}{}, fn(uow)
	})
	return err
}

// PostTransfer is the Read → Decide → Write sandwich, and the only place a
// movement is recorded.
//
//	Read   (impure): open the transaction, lock the touched accounts in
//	                 ascending id order (DDD-6), read the clock, generate the id
//	Decide (PURE):   domain.Post — validate, check I1 and I4, produce the
//	                 transaction, entries, and balance deltas
//	Write  (impure): persist all of it plus the idempotency record; commit
//
// The key and the transaction commit together, so there is no window in which
// one exists without the other (ADR-005).
func (l *Ledger) PostTransfer(ctx context.Context, cmd TransferRequest) (Result, error) {
	tenantID := cmd.TenantID
	uow, err := l.store.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("posting transfer: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	// Replay/conflict check (impure read, ahead of the Read → Decide → Write
	// sandwich): a same-key/same-fingerprint repeat is answered by
	// RE-RENDERING the stored transaction (DDD-8), never by re-running
	// domain.Post or serving a remembered response body. A same-key
	// different-fingerprint repeat is refused outright, here, before any
	// account is locked or any write attempted — the first transaction's
	// balances and entries are never touched by a conflicting reuse. The
	// unique constraint on the key (migration 0002) remains the backstop that
	// makes this hold under concurrent claims of the same key (I7); this
	// check is the correct behaviour on top of it, not a replacement for it.
	claim, found, err := uow.Idempotency().Lookup(ctx, cmd.IdempotencyKey)
	if err != nil {
		return Result{}, fmt.Errorf("looking up idempotency key for replay: %w", err)
	}
	if found {
		if claim.Fingerprint != cmd.Fingerprint {
			return Result{}, ErrIdempotencyKeyConflict
		}
		posting, err := uow.Transactions().Get(ctx, tenantID, claim.TransactionID)
		if err != nil {
			return Result{}, fmt.Errorf("re-rendering replay for transaction %q: %w", claim.TransactionID, err)
		}
		if err := uow.Commit(ctx); err != nil {
			return Result{}, fmt.Errorf("committing replay read for transaction %q: %w", claim.TransactionID, err)
		}
		committed = true
		return Result{Posting: posting, Replayed: true}, nil
	}

	// Read (impure): lock the touched accounts in ascending id order
	// (DDD-6) — the ordering is the repository's job, not this call site's.
	snapshots, err := uow.Accounts().LockForUpdate(ctx, tenantID, []string{cmd.From, cmd.To})
	if err != nil {
		return Result{}, fmt.Errorf("locking accounts for transfer: %w", err)
	}
	now := l.clock()
	transactionID := l.nextID()

	// Decide (pure): domain.Post is the whole rulebook. Nothing above or
	// below this line evaluates I1 or I4.
	posting, err := domain.Post(domain.TransferCommand{
		From:     cmd.From,
		To:       cmd.To,
		Amount:   cmd.Amount,
		TenantID: tenantID,
	}, snapshots, now, transactionID)
	if err != nil {
		return Result{}, err
	}

	// Write (impure): the transaction, its entries, the balance deltas, and
	// the idempotency claim all land in the same transaction, so there is no
	// window in which one exists without the others (ADR-005).
	if err := uow.Transactions().Append(ctx, tenantID, posting); err != nil {
		return Result{}, fmt.Errorf("recording transaction %q: %w", transactionID, err)
	}
	if err := uow.Accounts().ApplyDeltas(ctx, tenantID, posting.Deltas); err != nil {
		return Result{}, fmt.Errorf("applying balance deltas for transaction %q: %w", transactionID, err)
	}
	if _, err := uow.Idempotency().Claim(ctx, cmd.IdempotencyKey, cmd.Fingerprint, transactionID); err != nil {
		if errors.Is(err, ports.ErrIdempotencyKeyClaimConflict) {
			// Lost the race: another concurrent submission of this key
			// committed first. This attempt's transaction and entries were
			// never committed and are discarded by the deferred rollback
			// above (committed stays false) — nothing this attempt wrote
			// is ever visible. Re-resolving from the top re-runs the
			// Lookup/replay-or-conflict check above, which is now
			// guaranteed to find the winner's committed claim (I7), and
			// answers exactly as a same-key retry arriving after the
			// winner would.
			return l.PostTransfer(ctx, cmd)
		}
		return Result{}, fmt.Errorf("claiming idempotency key for transaction %q: %w", transactionID, err)
	}

	if err := uow.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("committing transaction %q: %w", transactionID, err)
	}
	committed = true

	return Result{Posting: posting, Replayed: false}, nil
}

// CreateAccount opens an account. A wallet starts at zero; value may only enter
// the ledger afterwards as a movement from a system account, never as an
// assignment — otherwise value appears unaccounted for and I1 is violated at
// the source.
func (l *Ledger) CreateAccount(ctx context.Context, tenantID string, accountID string, kind domain.AccountKind) error {
	openingBalance, err := domain.NewMoney(0, ledgerCurrency)
	if err != nil {
		return err
	}

	return inUnitOfWork(ctx, l.store, fmt.Sprintf("opening account %q", accountID), func(uow ports.UnitOfWork) error {
		// Courtesy check (DDD-18): read the snapshot before attempting the
		// insert. The unique constraint on the account name (migration 0) is
		// the final backstop under concurrency; this is the pure decision
		// that turns a known collision into a named refusal rather than a
		// raw constraint error.
		alreadyOpen, err := l.accountAlreadyOpen(ctx, uow, tenantID, accountID)
		if err != nil {
			return err
		}

		account, err := domain.OpenAccount(tenantID, accountID, kind, openingBalance, alreadyOpen)
		if err != nil {
			return err
		}

		if err := uow.Accounts().Create(ctx, tenantID, account); err != nil {
			return fmt.Errorf("opening account %q: %w", accountID, err)
		}
		return nil
	})
}

// accountAlreadyOpen performs the impure read behind the DDD-18 courtesy
// check: whether an account is already bound to this id. UnknownAccount is
// the expected shape of "no", not an error to propagate; anything else (a
// genuine infrastructure failure) is.
func (l *Ledger) accountAlreadyOpen(ctx context.Context, uow ports.UnitOfWork, tenantID, accountID string) (bool, error) {
	_, err := uow.Accounts().Get(ctx, tenantID, accountID)
	if err == nil {
		return true, nil
	}
	var violation domain.Violation
	if errors.As(err, &violation) && violation.Kind() == domain.UnknownAccount {
		return false, nil
	}
	return false, fmt.Errorf("checking whether account %q is already open: %w", accountID, err)
}

// ProvisionTenant mints a new tenant's identity and credential, and stores
// it — the Read → Decide → Write sandwich, one aggregate level up from
// CreateAccount, structurally identical on purpose (DDD-26):
//
//	Read   (impure): open the unit of work, look up whether name is already
//	                 bound (I10 courtesy check)
//	Decide (PURE):   domain.ProvisionTenant — refuses tenant_already_exists
//	                 before anything is written
//	Write  (impure): persist the tenant row (the adapter hashes the
//	                 credential; the plaintext never reaches storage)
//
// tenant_id and tenant_key are both minted via the existing IDGenerator port,
// tnt_/tk_-prefixed at this call site — no new randomness source (DDD-26).
// The plaintext tenant_key is returned to the caller here, in
// ProvisionedTenant, exactly once; nothing this function calls, and nothing
// downstream of it, retains it.
func (l *Ledger) ProvisionTenant(ctx context.Context, name string) (ProvisionedTenant, error) {
	return withUnitOfWork(ctx, l.store, fmt.Sprintf("provisioning tenant %q", name),
		func(uow ports.UnitOfWork) (ProvisionedTenant, error) {
			alreadyTaken, err := l.tenantNameAlreadyTaken(ctx, uow, name)
			if err != nil {
				return ProvisionedTenant{}, err
			}

			tenantID := "tnt_" + l.nextID()
			tenantKey := "tk_" + l.nextID()

			tenant, err := domain.ProvisionTenant(tenantID, name, tenantKey, alreadyTaken)
			if err != nil {
				return ProvisionedTenant{}, err
			}

			if err := uow.Tenants().Create(ctx, tenant); err != nil {
				return ProvisionedTenant{}, fmt.Errorf("provisioning tenant %q: %w", name, err)
			}

			return ProvisionedTenant{
				TenantID:  tenant.TenantID(),
				Name:      tenant.Name(),
				TenantKey: tenant.Credential(),
			}, nil
		})
}

// tenantNameAlreadyTaken performs the impure read behind the I10 courtesy
// check: whether a tenant is already bound to this name. TenantNotFound is
// the expected shape of "no", not an error to propagate; anything else (a
// genuine infrastructure failure) is — mirroring accountAlreadyOpen exactly,
// one aggregate level up.
func (l *Ledger) tenantNameAlreadyTaken(ctx context.Context, uow ports.UnitOfWork, name string) (bool, error) {
	_, err := uow.Tenants().ByName(ctx, name)
	if err == nil {
		return true, nil
	}
	var violation domain.Violation
	if errors.As(err, &violation) && violation.Kind() == domain.TenantNotFound {
		return false, nil
	}
	return false, fmt.Errorf("checking whether tenant name %q is already taken: %w", name, err)
}

// ProvisionedTenant is what a successful ProvisionTenant answers with. This
// is the one and only place TenantKey ever appears as plaintext — the
// caller (the HTTP handler, step 01-04) shows it to the operator once and
// retains it nowhere.
type ProvisionedTenant struct {
	TenantID  string
	Name      string
	TenantKey string
}

// GetBalance reads one account's stored balance, scoped to the caller's own
// tenant (step 02-04: tenantID arrives from the requireTenantKey-resolved
// scope, never a platform-wide read — GET /accounts/{id} has no unscoped
// mode).
func (l *Ledger) GetBalance(ctx context.Context, tenantID string, accountID string) (domain.Account, error) {
	return withUnitOfWork(ctx, l.store, fmt.Sprintf("reading balance for %q", accountID),
		func(uow ports.UnitOfWork) (domain.Account, error) {
			return uow.Accounts().Get(ctx, tenantID, accountID)
		})
}

// TracedEntry pairs one entry with the running balance immediately after it
// settles: the cumulative sum of Amount() over every entry up to and
// including this one, in the order EntriesFor already returns (recorded_at,
// then sequence). Entry.Amount() is already signed from the traced account's
// own perspective (fromDelta negative, toDelta positive), so running balance
// is a plain fold — no new sign logic belongs here.
type TracedEntry struct {
	Entry          domain.Entry
	RunningBalance domain.Money
}

// GetEntries reads one account's ordered history and folds each entry's
// signed amount into a running balance. Ordering is by recorded instant then
// by sequence, so two entries sharing a clock tick still read in a settled
// order (US-5).
//
// The account must already be open: tracing "nobody" answers
// domain.UnknownAccount naming the account, never a partial (or empty)
// result — the same account_not_found/404 decision site already sealed for
// POST /transfers and GET /accounts/{id} (ADR-008, DDD-17).
//
// scope arrives from GET /accounts/{id}/entries' dual-mode gate
// (requireTenantKeyOrOperatorKey, step 02-04): a tenant-key caller gets
// ScopedToTenant(their own tenant), an OperatorKey caller gets Unscoped().
// scope is passed straight through to EntriesFor — no translation — which is
// what keeps the unscoped path byte-identical to the pre-multitenancy query
// (step 02-02's own EntriesFor implementation already carries the unscoped
// SQL shape). The account-existence courtesy check runs either way: scoped
// via AccountRepository.Get (single-tenant read, as ports.go requires),
// unscoped via AccountRepository.ExistsAnyTenant — the one deliberately
// tenant-agnostic existence read (step 02-04), needed because an Unscoped()
// caller has no tenant of its own to filter by, yet "nobody" (an account no
// tenant ever opened) must still answer account_not_found/404, exactly as it
// did before multitenancy (ADR-008, DDD-17).
func (l *Ledger) GetEntries(ctx context.Context, scope ports.TenantScope, accountID string) ([]TracedEntry, error) {
	entries, err := withUnitOfWork(ctx, l.store, fmt.Sprintf("reading entries for %q", accountID),
		func(uow ports.UnitOfWork) ([]domain.Entry, error) {
			if tenantID, scoped := scope.Resolve(); scoped {
				if _, err := uow.Accounts().Get(ctx, tenantID, accountID); err != nil {
					return nil, err
				}
			} else {
				exists, err := uow.Accounts().ExistsAnyTenant(ctx, accountID)
				if err != nil {
					return nil, err
				}
				if !exists {
					return nil, domain.NewUnknownAccount(accountID)
				}
			}
			return uow.Transactions().EntriesFor(ctx, scope, accountID)
		})
	if err != nil {
		return nil, err
	}
	return runningBalances(entries)
}

// runningBalances is the pure fold at the heart of GetEntries: row i's
// running balance is the sum of Amount() over rows 0..i, in the order the
// entries already arrive. An empty history folds to an empty trace, never an
// error.
func runningBalances(entries []domain.Entry) ([]TracedEntry, error) {
	if len(entries) == 0 {
		return []TracedEntry{}, nil
	}

	running, err := zeroMoney(entries[0].Amount().Currency())
	if err != nil {
		return nil, err
	}

	traced := make([]TracedEntry, 0, len(entries))
	for _, entry := range entries {
		running, err = running.Add(entry.Amount())
		if err != nil {
			return nil, err
		}
		traced = append(traced, TracedEntry{Entry: entry, RunningBalance: running})
	}
	return traced, nil
}

// VerifyBooks answers the operator's one question by full scan (D9): sum every
// entry, group by account, and compare against the stored balances.
//
// scope decides whose books are scanned (step 03-01): Unscoped() reads
// exactly what it always has — legacyTenantID's own accounts (the
// pre-multitenancy accounts-enumeration leg is deliberately left unchanged
// here; see the comment on legacyTenantID) plus a platform-wide
// TrialBalance/ComputedBalances sum, byte-identical to the pre-feature
// contract. ScopedToTenant(id) instead narrows every leg — the accounts
// enumerated, the trial balance, and the computed balances — to that one
// tenant, and is refused tenant_not_found up front, before any of the three
// reads run, when id was never provisioned: TrialBalance/ComputedBalances
// alone cannot distinguish "unprovisioned tenant" from "provisioned tenant
// with nothing posted yet", so the existence check has to happen here,
// ahead of them, by naming.
//
// driftedAccounts (the pure comparison below) only ever sees what this
// function handed it — a tenant-scoped call's accounts and computed
// balances are already narrowed to that tenant by the two reads above, so a
// scoped verdict cannot name another tenant's drift by construction, not
// merely by convention.
//
// O(total history), deliberately. Checkpointing was deferred because the
// corruption demo modifies a HISTORICAL entry, which incremental verification
// would miss — any future checkpointing design must pair with tamper evidence
// rather than replace the full scan. ElapsedMillis exists so that degradation
// is measured rather than guessed at.
func (l *Ledger) VerifyBooks(ctx context.Context, scope ports.TenantScope) (BooksReport, error) {
	started := time.Now()

	report, err := withUnitOfWork(ctx, l.store, "verifying the books",
		func(uow ports.UnitOfWork) (BooksReport, error) {
			accountsTenantID := legacyTenantID
			if tenantID, scoped := scope.Resolve(); scoped {
				// Existence check FIRST (DDD-25): a name that was never
				// provisioned is refused before TrialBalance or
				// ComputedBalances ever runs, exactly as the design
				// context requires — "before any scan runs".
				if _, err := uow.Tenants().ByID(ctx, tenantID); err != nil {
					return BooksReport{}, err
				}
				accountsTenantID = tenantID
			}

			accounts, err := uow.Accounts().All(ctx, accountsTenantID)
			if err != nil {
				return BooksReport{}, fmt.Errorf("reading every account: %w", err)
			}
			computed, err := uow.Transactions().ComputedBalances(ctx, scope)
			if err != nil {
				return BooksReport{}, fmt.Errorf("computing balances from entries: %w", err)
			}
			trialBalance, entryCount, err := uow.Transactions().TrialBalance(ctx, scope)
			if err != nil {
				return BooksReport{}, fmt.Errorf("computing the trial balance: %w", err)
			}

			drifted, err := driftedAccounts(accounts, computed)
			if err != nil {
				return BooksReport{}, err
			}

			return BooksReport{
				Balanced:     len(drifted) == 0,
				TrialBalance: trialBalance,
				EntryCount:   entryCount,
				Drifted:      drifted,
			}, nil
		})
	if err != nil {
		return BooksReport{}, err
	}

	report.ElapsedMillis = int(time.Since(started).Milliseconds())
	return report, nil
}

// zeroMoney constructs currency's zero value — the shared starting point
// driftedAccounts uses for an account with no entries at all and
// runningBalances uses to seed its fold, so both pure folds read the same
// idiom for "nothing has happened yet" rather than reconstructing
// domain.NewMoney(0, ...) independently.
func zeroMoney(currency string) (domain.Money, error) {
	return domain.NewMoney(0, currency)
}

// driftedAccounts is the PURE comparison at the heart of VerifyBooks: for
// each stored account, compare its stored balance against what its entries
// sum to (I3), and report only the ones that disagree. An account with no
// entries at all computes to zero in its own currency, not to a missing map
// entry — a wallet that has never moved still has a defined computed balance.
func driftedAccounts(accounts []domain.Account, computed map[string]domain.Money) ([]Drift, error) {
	var drifted []Drift
	for _, account := range accounts {
		stored := account.Balance()

		balance, found := computed[account.ID()]
		if !found {
			zero, err := zeroMoney(stored.Currency())
			if err != nil {
				return nil, err
			}
			balance = zero
		}

		if balance == stored {
			continue
		}

		delta, err := balance.Add(stored.Negate())
		if err != nil {
			return nil, err
		}
		drifted = append(drifted, Drift{
			AccountID: account.ID(),
			Stored:    stored,
			Computed:  balance,
			Delta:     delta,
		})
	}
	return drifted, nil
}

// TransferRequest is a movement as it arrives from a driving adapter, with the
// caller-owned key attached. The key is required, never generated here
// (ADR-005; journey post-a-transfer, shared_artifacts).
type TransferRequest struct {
	From           string
	To             string
	Amount         domain.Money
	IdempotencyKey string
	Fingerprint    string

	// TenantID is the caller's own tenant, resolved by requireTenantKey
	// (step 02-04) — POST /transfers has no unscoped mode, unlike GET
	// /accounts/{id}/entries.
	TenantID string
}

// Result is what a posting answers with. Replayed distinguishes a retry from a
// first write, which is what lets the adapter answer 200 rather than 201
// without the caller having to tell them apart by content.
type Result struct {
	Posting  domain.Posting
	Replayed bool
}

// BooksReport is the operator's verdict. Drifted is empty on a healthy ledger,
// and every row names the account with its stored balance, its computed
// balance, and the delta — detection alone would pass a check and still fail
// the operator (KPI-4).
type BooksReport struct {
	Balanced      bool
	TrialBalance  domain.Money
	EntryCount    int
	ElapsedMillis int
	Drifted       []Drift
}

// Drift is one account whose stored balance disagrees with its entries (I3).
type Drift struct {
	AccountID string
	Stored    domain.Money
	Computed  domain.Money
	Delta     domain.Money
}
