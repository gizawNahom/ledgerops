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

// UnitOfWork is one posting's worth of transactional scope. The repositories
// below are reachable only through it, which is what stops them being wired
// to different transactions. Tenants() joined the other three as of step
// 01-03 (multitenancy): ProvisionTenant's I10 courtesy check (read existing
// names, decide, write) needs the same atomicity CreateAccount's I9 check
// already has, one aggregate level up (DDD-26).
type UnitOfWork interface {
	Accounts() AccountRepository
	Transactions() TransactionRepository
	Idempotency() IdempotencyStore
	Tenants() TenantRepository
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

// TenantRepository reads and creates tenant rows inside the same unit of
// work as ProvisionTenant's I10 courtesy check (read existing names, decide
// via the pure domain.ProvisionTenant constructor, write) — the same atomic
// check-then-create shape AccountRepository already gives CreateAccount one
// aggregate level up (DDD-26 reuse), mirrored deliberately rather than given
// a new persistence idiom.
type TenantRepository interface {
	// ByName reads a tenant by its display name, without locking. An absent
	// name returns domain.NewTenantNotFound, the expected shape of "no" —
	// mirroring AccountRepository.Get's UnknownAccount contract — not an
	// infrastructure error. Its only caller today is the I10 courtesy check;
	// the returned Tenant's Credential() is never meaningful (the plaintext
	// tenant_key cannot be recovered from its stored hash).
	ByName(ctx context.Context, name string) (domain.Tenant, error)
	// Create persists a newly provisioned tenant. tenant.Credential() carries
	// the plaintext tenant_key exactly as domain.ProvisionTenant produced it
	// — the adapter hashes it (sha256, hex-encoded) before it ever reaches a
	// column or a WHERE clause; the domain and this port never see the hash.
	Create(ctx context.Context, tenant domain.Tenant) error
}

// TenantKeyResolver resolves a presented bearer token's SHA-256 hash to the
// tenant_id it belongs to. It is the pure-function, return-only contract
// shape (2026-05-15 mandate): a function type, not an interface, so a
// second operation — a "touch last-seen" write, say — is structurally
// impossible to add here, not merely disciplined against. Called directly by
// the HTTP auth middleware (a later step, 02-03) ahead of any Ledger use
// case; this step declares the type and implements its PostgreSQL-backed
// resolver, nothing more.
type TenantKeyResolver func(ctx context.Context, credentialHash string) (tenantID string, ok bool, err error)
