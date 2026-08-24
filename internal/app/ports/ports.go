// Package ports declares the effects the pure core refuses to perform. Under
// the functional paradigm the ports-and-adapters boundary and the purity
// boundary are the same line: a port is the type of an effect.
//
// Ports are hybrid by arity (DDD-13). A port with one operation is a function
// type, so its fake is a one-line literal and no test needs a stub. A port
// whose operations must share a transaction handle stays an interface, because
// as loose function types a caller could wire two of them to different
// transactions and nothing in the type system would object — which is exactly
// the class of bug DDD-6's lock-ordering rule exists to prevent.
//
// Every port declared here is implemented for real: internal/adapters/postgres
// backs Store/UnitOfWork/AccountRepository/TransactionRepository/
// IdempotencyStore, and internal/app wires Clock/IDGenerator. There are no
// scaffold bodies left in this file.
package ports

import (
	"context"
	"errors"
	"time"

	"ledgerops/internal/domain"
)

// Clock is a single, independent effect, so it is a function type.
type Clock func() time.Time

// IDGenerator is likewise a single effect. Both exist as ports purely for
// testability — that is the second-ranked quality attribute doing visible work.
type IDGenerator func() string

// UnitOfWork is one posting's worth of transactional scope. The three
// repositories below are reachable only through it, which is what stops two of
// them being wired to different transactions.
type UnitOfWork interface {
	Accounts() AccountRepository
	Transactions() TransactionRepository
	Idempotency() IdempotencyStore
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Store opens a unit of work. It is the only thing the application layer holds.
type Store interface {
	Begin(ctx context.Context) (UnitOfWork, error)
	Close() error
}

// AccountRepository locks and reads the accounts a posting touches, and applies
// the balance deltas the pure core returned.
//
// LockForUpdate MUST acquire its locks in ascending account-id order (DDD-6),
// regardless of the order the caller passes them in. That ordering rule lives
// here, in the shell — locking is an effect, and the pure core neither knows
// nor could express it.
type AccountRepository interface {
	LockForUpdate(ctx context.Context, accountIDs []string) ([]domain.Account, error)
	ApplyDeltas(ctx context.Context, deltas []domain.BalanceDelta) error
	Create(ctx context.Context, account domain.Account) error
	Get(ctx context.Context, accountID string) (domain.Account, error)
	// All enumerates every account the ledger has ever opened, in a
	// deterministic order. VerifyBooks (D9) is the one caller: its full-scan
	// verdict needs every stored balance to compare against ComputedBalances,
	// not just the ones a caller happens to ask about.
	All(ctx context.Context) ([]domain.Account, error)
}

// TransactionRepository writes the transaction and its entries. It offers no
// update and no delete, and the store refuses both anyway (D7 / OPS-10) — the
// absence here is a reminder, not the enforcement.
type TransactionRepository interface {
	Append(ctx context.Context, posting domain.Posting) error
	Get(ctx context.Context, transactionID string) (domain.Posting, error)
	EntriesFor(ctx context.Context, accountID string) ([]domain.Entry, error)
	TrialBalance(ctx context.Context) (domain.Money, int, error)
	ComputedBalances(ctx context.Context) (map[string]domain.Money, error)
}

// IdempotencyStore records the key, the request fingerprint, and the resulting
// transaction id, inside the same transaction as the posting it guards
// (ADR-005). The unique constraint on the key is what makes I7 hold under
// concurrency — not the code around it.
type IdempotencyStore interface {
	Claim(ctx context.Context, key string, fingerprint string, transactionID string) (Claim, error)
	Lookup(ctx context.Context, key string) (Claim, bool, error)
}

// ErrIdempotencyKeyClaimConflict marks a Claim call that lost a race: another
// concurrent request's Claim for the same key committed first. The unique
// constraint on the key (I7) is what makes this detectable rather than a
// silent double-write. The caller must abandon whatever this attempt wrote —
// never commit it — and re-resolve the key, exactly as a same-key retry
// arriving after the winner's commit would (ADR-005).
var ErrIdempotencyKeyClaimConflict = errors.New("idempotency key claimed concurrently by another request")

// Claim is what an idempotency record holds. There is no cached response body:
// replays re-render from the stored transaction (ADR-005), so a later change to
// response format cannot leave replays serving the old shape indefinitely.
type Claim struct {
	Key           string
	Fingerprint   string
	TransactionID string
}
