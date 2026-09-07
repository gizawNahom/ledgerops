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
// already has, one aggregate level up (DDD-26). TenantLinks() joins them as
// of step 01-03 (inter-tenant-transfer): AuthorizeTenantPair's I11 courtesy
// check (read the active link for a pair, decide via the pure domain
// constructor, write) needs that same atomicity, one aggregate over again.
type UnitOfWork interface {
	Accounts() AccountRepository
	Transactions() TransactionRepository
	Idempotency() IdempotencyStore
	Tenants() TenantRepository
	TenantLinks() TenantLinkRepository
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Store opens a unit of work. It is the only thing the application layer holds.
type Store interface {
	Begin(ctx context.Context) (UnitOfWork, error)
	Close() error
}

// TenantScope is the closed, two-constructor credential-mechanism type for
// the read-only ports where a legitimate unscoped state exists
// (brief.md § Application Architecture / Multitenancy). It is NOT a
// nullable string: the only ways to produce one are ScopedToTenant and
// Unscoped, and the only way to read one back is Resolve, so "forgot to
// scope" and "deliberately unscoped" cannot be confused with each other or
// represented by an accidental zero value.
//
// Used only by EntriesFor, TrialBalance, and ComputedBalances — the three
// read-only ports where a platform-wide aggregate is a real, intentional
// answer. Every write-path port and every other read keeps tenant_id as a
// plain required string; there is no legitimate unscoped write.
type TenantScope struct {
	tenantID string
	scoped   bool
}

// ScopedToTenant returns a scope naming exactly one tenant.
func ScopedToTenant(tenantID string) TenantScope {
	return TenantScope{tenantID: tenantID, scoped: true}
}

// Unscoped returns the deliberate platform-wide scope — not a missing value,
// a chosen one.
func Unscoped() TenantScope {
	return TenantScope{}
}

// Resolve reads the scope back: tenantID is meaningful only when scoped is
// true.
func (s TenantScope) Resolve() (tenantID string, scoped bool) {
	return s.tenantID, s.scoped
}

// AccountRepository locks and reads the accounts a posting touches, and applies
// the balance deltas the pure core returned.
//
// Every operation takes tenantID as a required, plain string — never
// TenantScope — because there is no legitimate unscoped state for a write or
// for a single-tenant read (brief.md § Multitenancy Contract-shape table).
// Account-name uniqueness (I9/DDD-18) is scoped to (tenant_id, id), not
// global id alone, as of this port.
//
// LockForUpdate MUST acquire its locks in ascending account-id order (DDD-6),
// scoped within the given tenant's own account set, regardless of the order
// the caller passes them in. That ordering rule lives here, in the shell —
// locking is an effect, and the pure core neither knows nor could express it.
type AccountRepository interface {
	LockForUpdate(ctx context.Context, tenantID string, accountIDs []string) ([]domain.Account, error)
	ApplyDeltas(ctx context.Context, tenantID string, deltas []domain.BalanceDelta) error
	Create(ctx context.Context, tenantID string, account domain.Account) error
	Get(ctx context.Context, tenantID string, accountID string) (domain.Account, error)
	// All enumerates every account the given tenant has ever opened, in a
	// deterministic order. VerifyBooks (D9) is the one caller: its full-scan
	// verdict needs every stored balance to compare against ComputedBalances,
	// not just the ones a caller happens to ask about.
	All(ctx context.Context, tenantID string) ([]domain.Account, error)

	// ExistsAnyTenant reports whether an account with this id exists under
	// ANY tenant. This is deliberately the one account-identity read in this
	// port that is NOT scoped to a caller-named tenant -- it exists solely
	// for GetEntries' dual-mode courtesy check (step 02-04): an Unscoped()
	// OperatorKey caller has no tenant of its own to filter existence by,
	// yet the account_not_found/404 contract for a truly nonexistent account
	// (ADR-008, DDD-17) must still hold, exactly as it did before
	// multitenancy. It answers only "does a row exist", never which tenant
	// owns it or what it contains -- narrower than a read, not an unscoped
	// version of Get.
	ExistsAnyTenant(ctx context.Context, accountID string) (bool, error)
}

// TransactionRepository writes the transaction and its entries. It offers no
// update and no delete, and the store refuses both anyway (D7 / OPS-10) — the
// absence here is a reminder, not the enforcement.
//
// Append and Get are write-adjacent (a transaction has exactly one owning
// tenant) and take tenantID as a plain required string. EntriesFor,
// TrialBalance, and ComputedBalances are the pure-function, return-only
// reads where a platform-wide aggregate is a legitimate answer, so they take
// a closed TenantScope instead (brief.md § Multitenancy Contract-shape
// table).
type TransactionRepository interface {
	Append(ctx context.Context, tenantID string, posting domain.Posting) error
	Get(ctx context.Context, tenantID string, transactionID string) (domain.Posting, error)
	EntriesFor(ctx context.Context, scope TenantScope, accountID string) ([]domain.Entry, error)
	TrialBalance(ctx context.Context, scope TenantScope) (domain.Money, int, error)
	ComputedBalances(ctx context.Context, scope TenantScope) (map[string]domain.Money, error)
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
	// ByID reads a tenant by its tenant_id, without locking. An absent id
	// answers domain.NewTenantNotFound, the expected shape of "no" —
	// mirroring ByName's own contract, one lookup key over. Its caller is
	// VerifyBooks' tenant-scoped existence check (step 03-01): the
	// trial-balance/computed-balance reads alone cannot tell "unprovisioned
	// tenant_id" apart from "provisioned tenant with no entries yet", so the
	// refusal has to be decided here, by naming, before either read runs.
	ByID(ctx context.Context, tenantID string) (domain.Tenant, error)
	// Create persists a newly provisioned tenant. tenant.Credential() carries
	// the plaintext tenant_key exactly as domain.ProvisionTenant produced it
	// — the adapter hashes it (sha256, hex-encoded) before it ever reaches a
	// column or a WHERE clause; the domain and this port never see the hash.
	Create(ctx context.Context, tenant domain.Tenant) error
}

// TenantLinkRepository reads and writes standing tenant-pair authorizations
// inside the same unit of work as AuthorizeTenantPair's I11 courtesy check
// (read the active link for a pair, decide via the pure
// domain.AuthorizeTenantPair constructor, write) — the same atomic
// read-then-decide-then-write shape TenantRepository already gives
// ProvisionTenant one aggregate over (DDD-26 reuse), mirrored deliberately
// rather than given a new persistence idiom.
type TenantLinkRepository interface {
	// Create persists a newly authorized (or freshly re-authorized) link.
	// tenant_a/tenant_b are already canonicalized by the pure
	// domain.AuthorizeTenantPair constructor before this is ever called —
	// the adapter stores them exactly as handed, transforming nothing
	// (mirrors tenants.go's own division of labor).
	Create(ctx context.Context, link domain.TenantLink) error

	// ActiveByPair reads the active link, if any, for an unordered tenant
	// pair — callers may name the pair in either order; canonicalization on
	// write means the adapter checks both directions rather than requiring
	// a pre-canonicalized caller. Mirrors IdempotencyStore.Lookup's
	// (value, found, error) shape: an absent active link is the expected
	// shape of "no", not an error. Used both by AuthorizeTenantPair's
	// existence check and, later (step 02-04), by
	// RegisterCounterpartyAlias.
	ActiveByPair(ctx context.Context, tenantA, tenantB string) (domain.TenantLink, bool, error)

	// ByID reads a link by its link_id, without locking. An absent id
	// answers domain.NewTenantLinkNotFound, mirroring TenantRepository.ByID's
	// own absent-row contract — the expected shape of "no", not an
	// infrastructure error. RevokeTenantLink's use case is the only caller
	// today.
	ByID(ctx context.Context, linkID string) (domain.TenantLink, error)

	// Revoke transitions the named link's status to revoked in place — the
	// one deliberate in-place mutation this schema grants (migration 0004),
	// unlike the append-only ledger tables. Naming an id absent from the
	// table answers domain.NewTenantLinkNotFound, the same expected shape of
	// "no" ByID and RevokeTenantLink's own pure decision already use.
	Revoke(ctx context.Context, linkID string) error
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
