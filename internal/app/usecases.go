// Package app is the effect shell. It sequences read, decide, and write, and
// holds every ordering rule the pure core cannot express: lock acquisition
// order, transaction demarcation, and idempotency-key insertion.
//
// The dependency rule: the shell may call the core, the core never calls the
// shell, and the core does not know the shell exists.
//
// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
package app

import (
	"context"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

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
	panic("NewLedger not yet implemented -- RED scaffold")
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
	panic("Ledger.PostTransfer not yet implemented -- RED scaffold")
}

// CreateAccount opens an account. A wallet starts at zero; value may only enter
// the ledger afterwards as a movement from a system account, never as an
// assignment — otherwise value appears unaccounted for and I1 is violated at
// the source.
func (l *Ledger) CreateAccount(ctx context.Context, accountID string, kind domain.AccountKind) error {
	panic("Ledger.CreateAccount not yet implemented -- RED scaffold")
}

// GetBalance reads one account's stored balance.
func (l *Ledger) GetBalance(ctx context.Context, accountID string) (domain.Account, error) {
	panic("Ledger.GetBalance not yet implemented -- RED scaffold")
}

// GetEntries reads one account's ordered history. Ordering is by recorded
// instant then by sequence, so two entries sharing a clock tick still read in a
// settled order (US-5).
func (l *Ledger) GetEntries(ctx context.Context, accountID string) ([]domain.Entry, error) {
	panic("Ledger.GetEntries not yet implemented -- RED scaffold")
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
