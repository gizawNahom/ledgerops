// Package app is the effect shell. It sequences read, decide, and write, and
// holds every ordering rule the pure core cannot express: lock acquisition
// order, transaction demarcation, and idempotency-key insertion.
//
// The dependency rule: the shell may call the core, the core never calls the
// shell, and the core does not know the shell exists.
//
// PostTransfer, CreateAccount, and GetBalance are real as of step 01-03: the
// Read → Decide → Write sandwich over the real postgres repositories, one
// database transaction per call. GetEntries is real as of step 02-01,
// narrowly: it reads the ordered history straight through to the wire so a
// posted transfer's two legs can be traced to one transaction id; the full
// traceability contract (running balance, unknown-account refusal) is
// milestone-05's job. VerifyBooks remains a RED scaffold — out of scope for
// this step (slice 04).
package app

import (
	"context"
	"errors"
	"fmt"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// ledgerCurrency is the ledger's single configured currency. Every account is
// opened in it; a movement across currencies never reaches this shell because
// nothing here ever constructs a second one (see domain.CurrencyMismatch —
// currently unreachable through any driving port, by design).
const ledgerCurrency = "USD"

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

	// Replay check (impure read, ahead of the Read → Decide → Write
	// sandwich): a same-key/same-fingerprint repeat is answered by
	// RE-RENDERING the stored transaction (DDD-8), never by re-running
	// domain.Post or serving a remembered response body. A same-key
	// different-fingerprint repeat is NOT handled here — it falls through to
	// the normal write path, where the unique constraint on the key is the
	// backstop (key-conflict refusal is a later step's job).
	claim, found, err := uow.Idempotency().Lookup(ctx, cmd.IdempotencyKey)
	if err != nil {
		return Result{}, fmt.Errorf("looking up idempotency key for replay: %w", err)
	}
	if found && claim.Fingerprint == cmd.Fingerprint {
		posting, err := uow.Transactions().Get(ctx, claim.TransactionID)
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
	snapshots, err := uow.Accounts().LockForUpdate(ctx, []string{cmd.From, cmd.To})
	if err != nil {
		return Result{}, fmt.Errorf("locking accounts for transfer: %w", err)
	}
	now := l.clock()
	transactionID := l.nextID()

	// Decide (pure): domain.Post is the whole rulebook. Nothing above or
	// below this line evaluates I1 or I4.
	posting, err := domain.Post(domain.TransferCommand{
		From:   cmd.From,
		To:     cmd.To,
		Amount: cmd.Amount,
	}, snapshots, now, transactionID)
	if err != nil {
		return Result{}, err
	}

	// Write (impure): the transaction, its entries, the balance deltas, and
	// the idempotency claim all land in the same transaction, so there is no
	// window in which one exists without the others (ADR-005).
	if err := uow.Transactions().Append(ctx, posting); err != nil {
		return Result{}, fmt.Errorf("recording transaction %q: %w", transactionID, err)
	}
	if err := uow.Accounts().ApplyDeltas(ctx, posting.Deltas); err != nil {
		return Result{}, fmt.Errorf("applying balance deltas for transaction %q: %w", transactionID, err)
	}
	if _, err := uow.Idempotency().Claim(ctx, cmd.IdempotencyKey, cmd.Fingerprint, transactionID); err != nil {
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
func (l *Ledger) CreateAccount(ctx context.Context, accountID string, kind domain.AccountKind) error {
	uow, err := l.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening account %q: %w", accountID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	zero, err := domain.NewMoney(0, ledgerCurrency)
	if err != nil {
		return err
	}

	// Courtesy check (DDD-18): read the snapshot before attempting the
	// insert. The unique constraint on the account name (migration 0) is the
	// final backstop under concurrency; this is the pure decision that turns
	// a known collision into a named refusal rather than a raw constraint
	// error.
	alreadyOpen, err := l.accountAlreadyOpen(ctx, uow, accountID)
	if err != nil {
		return err
	}

	account, err := domain.OpenAccount(accountID, kind, zero, alreadyOpen)
	if err != nil {
		return err
	}

	if err := uow.Accounts().Create(ctx, account); err != nil {
		return fmt.Errorf("opening account %q: %w", accountID, err)
	}
	if err := uow.Commit(ctx); err != nil {
		return fmt.Errorf("opening account %q: %w", accountID, err)
	}
	committed = true
	return nil
}

// accountAlreadyOpen performs the impure read behind the DDD-18 courtesy
// check: whether an account is already bound to this id. UnknownAccount is
// the expected shape of "no", not an error to propagate; anything else (a
// genuine infrastructure failure) is.
func (l *Ledger) accountAlreadyOpen(ctx context.Context, uow ports.UnitOfWork, accountID string) (bool, error) {
	_, err := uow.Accounts().Get(ctx, accountID)
	if err == nil {
		return true, nil
	}
	var violation domain.Violation
	if errors.As(err, &violation) && violation.Kind() == domain.UnknownAccount {
		return false, nil
	}
	return false, fmt.Errorf("checking whether account %q is already open: %w", accountID, err)
}

// GetBalance reads one account's stored balance.
func (l *Ledger) GetBalance(ctx context.Context, accountID string) (domain.Account, error) {
	uow, err := l.store.Begin(ctx)
	if err != nil {
		return domain.Account{}, fmt.Errorf("reading balance for %q: %w", accountID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	account, err := uow.Accounts().Get(ctx, accountID)
	if err != nil {
		return domain.Account{}, err
	}
	if err := uow.Commit(ctx); err != nil {
		return domain.Account{}, fmt.Errorf("reading balance for %q: %w", accountID, err)
	}
	committed = true
	return account, nil
}

// GetEntries reads one account's ordered history. Ordering is by recorded
// instant then by sequence, so two entries sharing a clock tick still read in a
// settled order (US-5).
func (l *Ledger) GetEntries(ctx context.Context, accountID string) ([]domain.Entry, error) {
	uow, err := l.store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading entries for %q: %w", accountID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = uow.Rollback(ctx)
		}
	}()

	entries, err := uow.Transactions().EntriesFor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := uow.Commit(ctx); err != nil {
		return nil, fmt.Errorf("reading entries for %q: %w", accountID, err)
	}
	committed = true
	return entries, nil
}

// VerifyBooks answers the operator's one question by full scan (D9): sum every
// entry, group by account, and compare against the stored balances.
//
// O(total history), deliberately. Checkpointing was deferred because the
// corruption demo modifies a HISTORICAL entry, which incremental verification
// would miss — any future checkpointing design must pair with tamper evidence
// rather than replace the full scan. ElapsedMillis exists so that degradation
// is measured rather than guessed at.
func (l *Ledger) VerifyBooks(ctx context.Context) (BooksReport, error) {
	panic("Ledger.VerifyBooks not yet implemented -- RED scaffold")
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
