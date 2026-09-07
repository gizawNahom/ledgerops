package app_test

// Fakes for the three transactional ports (Store/UnitOfWork/
// AccountRepository/TransactionRepository/IdempotencyStore), permitted at the
// application layer per DDD-13's Clock/IDGenerator split and the step 01-03
// TEST PARADIGM note: PostTransfer's ORCHESTRATION is what this suite proves,
// with the real repositories themselves exercised for real via Testcontainers
// in internal/adapters/postgres (Mandate 6). A fake repository here would be
// modelling the very transactional behaviour that package proves for real.
//
// Per the test-double-input-validation doctrine (nw-tdd-methodology), these
// fakes reject what the real repositories would reject: ApplyDeltas on an
// unknown account, Create on a duplicate id, Claim on an already-claimed key.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// fakeStore is the shared state a fakeUnitOfWork's repositories read and
// write. One fakeStore per test case — never shared across rapid.Check
// iterations — so before/after snapshots are never contaminated by a prior
// draw.
type fakeStore struct {
	accounts          map[string]domain.Account
	postings          map[string]domain.Posting
	entries           []domain.Entry
	claims            map[string]ports.Claim
	tenants           map[string]domain.Tenant
	tenantLinks       map[string]domain.TenantLink
	counterpartyAlias map[string]domain.CounterpartyAlias
	transferStates    map[string]ports.TransferState
	committed         bool

	// scanAttempted and lastScope are spies for step 03-01's VerifyBooks
	// wiring: scanAttempted proves the tenant_not_found refusal path never
	// reaches TrialBalance/ComputedBalances, and lastScope proves whatever
	// TenantScope VerifyBooks received is threaded through unchanged, never
	// widened back to Unscoped().
	scanAttempted bool
	lastScope     ports.TenantScope

	// lastAccountsAllTenantID is the tenantID Accounts().All was last called
	// with — the other half of VerifyBooks' wiring proof: a tenant-scoped
	// call must enumerate that tenant's own accounts, not legacyTenantID.
	lastAccountsAllTenantID string
}

func newFakeStore(accounts ...domain.Account) *fakeStore {
	byID := make(map[string]domain.Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID()] = account
	}
	return &fakeStore{
		accounts:          byID,
		postings:          map[string]domain.Posting{},
		claims:            map[string]ports.Claim{},
		tenants:           map[string]domain.Tenant{},
		tenantLinks:       map[string]domain.TenantLink{},
		counterpartyAlias: map[string]domain.CounterpartyAlias{},
		transferStates:    map[string]ports.TransferState{},
	}
}

func (s *fakeStore) Begin(ctx context.Context) (ports.UnitOfWork, error) {
	return &fakeUnitOfWork{store: s}, nil
}

func (s *fakeStore) Close() error { return nil }

type fakeUnitOfWork struct {
	store      *fakeStore
	rolledBack bool
}

func (u *fakeUnitOfWork) Accounts() ports.AccountRepository { return fakeAccountRepository{u.store} }
func (u *fakeUnitOfWork) Transactions() ports.TransactionRepository {
	return fakeTransactionRepository{u.store}
}
func (u *fakeUnitOfWork) Idempotency() ports.IdempotencyStore { return fakeIdempotencyStore{u.store} }
func (u *fakeUnitOfWork) Tenants() ports.TenantRepository     { return fakeTenantRepository{u.store} }
func (u *fakeUnitOfWork) TenantLinks() ports.TenantLinkRepository {
	return fakeTenantLinkRepository{u.store}
}

// CounterpartyAliases and TransferStates joined the other fakes as of step
// 02-03 (inter-tenant-transfer): ports.UnitOfWork now requires both of
// every implementer, this fake included — same reason
// fakeTenantLinkRepository joined at step 01-03.
func (u *fakeUnitOfWork) CounterpartyAliases() ports.CounterpartyAliasRepository {
	return fakeCounterpartyAliasRepository{u.store}
}

func (u *fakeUnitOfWork) TransferStates() ports.TransferStateRepository {
	return fakeTransferStateRepository{u.store}
}

func (u *fakeUnitOfWork) Commit(ctx context.Context) error {
	u.store.committed = true
	return nil
}

func (u *fakeUnitOfWork) Rollback(ctx context.Context) error {
	u.rolledBack = true
	return nil
}

type fakeAccountRepository struct{ store *fakeStore }

var _ ports.AccountRepository = fakeAccountRepository{}

func (r fakeAccountRepository) LockForUpdate(ctx context.Context, tenantID string, accountIDs []string) ([]domain.Account, error) {
	seen := make(map[string]bool, len(accountIDs))
	sorted := make([]string, 0, len(accountIDs))
	for _, id := range accountIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	accounts := make([]domain.Account, 0, len(sorted))
	for _, id := range sorted {
		if account, ok := r.store.accounts[id]; ok {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r fakeAccountRepository) ApplyDeltas(ctx context.Context, tenantID string, deltas []domain.BalanceDelta) error {
	for _, delta := range deltas {
		account, ok := r.store.accounts[delta.AccountID]
		if !ok {
			return fmt.Errorf("fakeAccountRepository: unknown account %q", delta.AccountID)
		}
		updated, err := account.Apply(delta.Delta)
		if err != nil {
			return err
		}
		r.store.accounts[delta.AccountID] = updated
	}
	return nil
}

func (r fakeAccountRepository) Create(ctx context.Context, tenantID string, account domain.Account) error {
	if _, exists := r.store.accounts[account.ID()]; exists {
		return fmt.Errorf("fakeAccountRepository: account %q already exists", account.ID())
	}
	r.store.accounts[account.ID()] = account
	return nil
}

func (r fakeAccountRepository) Get(ctx context.Context, tenantID string, accountID string) (domain.Account, error) {
	account, ok := r.store.accounts[accountID]
	if !ok {
		return domain.Account{}, domain.NewUnknownAccount(accountID)
	}
	return account, nil
}

// All enumerates every account, ordered by id — VerifyBooks' full-scan
// contract (D9) is what this fake exists for.
//
// tenantID is accepted, not enforced: every call this suite issues goes
// through app.Ledger, which always resolves to the same tenant internally
// (step 02-02's legacyTenantID, pending 02-04's real extraction) — so a
// single-tenant map is a faithful stand-in for PostTransfer/CreateAccount/
// VerifyBooks' ORCHESTRATION, which is what this suite proves. Tenant
// ISOLATION itself is proven for real against PostgreSQL (WS strategy C,
// docs/feature/multitenancy/feature-delta.md): a fake enforcing scoping here
// would model the very behaviour that suite exists to check.
func (r fakeAccountRepository) All(ctx context.Context, tenantID string) ([]domain.Account, error) {
	r.store.lastAccountsAllTenantID = tenantID
	ids := make([]string, 0, len(r.store.accounts))
	for id := range r.store.accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	accounts := make([]domain.Account, 0, len(ids))
	for _, id := range ids {
		accounts = append(accounts, r.store.accounts[id])
	}
	return accounts, nil
}

// ExistsAnyTenant mirrors the real adapter's tenant-agnostic existence read
// (step 02-04): this fake's map is already single-tenant-shaped (see All's
// comment above), so a plain map lookup is a faithful stand-in.
func (r fakeAccountRepository) ExistsAnyTenant(ctx context.Context, accountID string) (bool, error) {
	_, ok := r.store.accounts[accountID]
	return ok, nil
}

type fakeTransactionRepository struct{ store *fakeStore }

var _ ports.TransactionRepository = fakeTransactionRepository{}

func (r fakeTransactionRepository) Append(ctx context.Context, tenantID string, posting domain.Posting) error {
	if _, exists := r.store.postings[posting.Transaction.ID()]; exists {
		return fmt.Errorf("fakeTransactionRepository: transaction %q already recorded", posting.Transaction.ID())
	}
	r.store.postings[posting.Transaction.ID()] = posting
	r.store.entries = append(r.store.entries, posting.Entries...)
	return nil
}

func (r fakeTransactionRepository) Get(ctx context.Context, tenantID string, transactionID string) (domain.Posting, error) {
	posting, ok := r.store.postings[transactionID]
	if !ok {
		return domain.Posting{}, fmt.Errorf("fakeTransactionRepository: unknown transaction %q", transactionID)
	}
	return posting, nil
}

func (r fakeTransactionRepository) EntriesFor(ctx context.Context, scope ports.TenantScope, accountID string) ([]domain.Entry, error) {
	var entries []domain.Entry
	for _, entry := range r.store.entries {
		if entry.AccountID() == accountID {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// TrialBalance sums every entry, matching the real repository's contract
// (internal/adapters/postgres/transactions.go) closely enough for VerifyBooks'
// orchestration to be exercised over this fake — an all-zero stand-in would
// hide the very defect (a stored balance that disagrees with its entries)
// VerifyBooks exists to catch.
func (r fakeTransactionRepository) TrialBalance(ctx context.Context, scope ports.TenantScope) (domain.Money, int, error) {
	r.store.scanAttempted = true
	r.store.lastScope = scope
	currency := "USD"
	var sumMinor int64
	for _, entry := range r.store.entries {
		sumMinor += entry.Amount().MinorUnits()
		currency = entry.Amount().Currency()
	}
	total, err := domain.NewMoney(sumMinor, currency)
	if err != nil {
		return domain.Money{}, 0, err
	}
	return total, len(r.store.entries), nil
}

// ComputedBalances derives every account's balance from its entries, grouped
// by account — the other half of VerifyBooks' I3 comparison.
func (r fakeTransactionRepository) ComputedBalances(ctx context.Context, scope ports.TenantScope) (map[string]domain.Money, error) {
	r.store.scanAttempted = true
	r.store.lastScope = scope
	sums := map[string]int64{}
	currencies := map[string]string{}
	for _, entry := range r.store.entries {
		sums[entry.AccountID()] += entry.Amount().MinorUnits()
		currencies[entry.AccountID()] = entry.Amount().Currency()
	}
	balances := make(map[string]domain.Money, len(sums))
	for accountID, sum := range sums {
		balance, err := domain.NewMoney(sum, currencies[accountID])
		if err != nil {
			return nil, err
		}
		balances[accountID] = balance
	}
	return balances, nil
}

type fakeIdempotencyStore struct{ store *fakeStore }

var _ ports.IdempotencyStore = fakeIdempotencyStore{}

func (r fakeIdempotencyStore) Claim(ctx context.Context, key, fingerprint, transactionID string) (ports.Claim, error) {
	if existing, ok := r.store.claims[key]; ok {
		return ports.Claim{}, fmt.Errorf("fakeIdempotencyStore: key %q already claimed by transaction %q", key, existing.TransactionID)
	}
	claim := ports.Claim{Key: key, Fingerprint: fingerprint, TransactionID: transactionID}
	r.store.claims[key] = claim
	return claim, nil
}

func (r fakeIdempotencyStore) Lookup(ctx context.Context, key string) (ports.Claim, bool, error) {
	claim, ok := r.store.claims[key]
	return claim, ok, nil
}

// fakeTenantRepository validates like the real tenantRepository
// (nw-tdd-methodology's test-double-input-validation doctrine): ByName
// answers TenantNotFound for an absent name, exactly as
// postgres.tenantRepository.ByName does, and Create rejects a name already
// bound so a fake covering ProvisionTenant's I10 refusal path never diverges
// from the real adapter's behaviour under the unique constraint.
type fakeTenantRepository struct{ store *fakeStore }

var _ ports.TenantRepository = fakeTenantRepository{}

func (r fakeTenantRepository) ByName(ctx context.Context, name string) (domain.Tenant, error) {
	tenant, ok := r.store.tenants[name]
	if !ok {
		return domain.Tenant{}, domain.NewTenantNotFound(name)
	}
	return tenant, nil
}

// ByID mirrors the real tenantRepository.ByID contract: an absent id answers
// domain.NewTenantNotFound, not an infrastructure error. The fake's store is
// keyed by name, so this is a linear scan rather than a second index — the
// tenant counts these tests ever seed are small enough that this stays a
// faithful, boring stand-in rather than a shortcut that could hide a bug.
func (r fakeTenantRepository) ByID(ctx context.Context, tenantID string) (domain.Tenant, error) {
	for _, tenant := range r.store.tenants {
		if tenant.TenantID() == tenantID {
			return tenant, nil
		}
	}
	return domain.Tenant{}, domain.NewTenantNotFound(tenantID)
}

func (r fakeTenantRepository) Create(ctx context.Context, tenant domain.Tenant) error {
	if _, exists := r.store.tenants[tenant.Name()]; exists {
		return fmt.Errorf("fakeTenantRepository: tenant name %q already exists", tenant.Name())
	}
	r.store.tenants[tenant.Name()] = tenant
	return nil
}

// fakeTenantLinkRepository joined the other fakes as of step 01-03
// (inter-tenant-transfer): AuthorizeTenantPair/RevokeTenantLink need it, and
// ports.UnitOfWork now requires TenantLinks() of every implementer, this
// fake included. ActiveByPair mirrors the real adapter's pair-direction
// tolerance: a caller may name the pair in either order and still find the
// same stored (already-canonicalized) row.
type fakeTenantLinkRepository struct{ store *fakeStore }

var _ ports.TenantLinkRepository = fakeTenantLinkRepository{}

func (r fakeTenantLinkRepository) Create(ctx context.Context, link domain.TenantLink) error {
	r.store.tenantLinks[link.LinkID()] = link
	return nil
}

func (r fakeTenantLinkRepository) ActiveByPair(ctx context.Context, tenantA, tenantB string) (domain.TenantLink, bool, error) {
	for _, link := range r.store.tenantLinks {
		if link.Status() != domain.TenantLinkActive {
			continue
		}
		matchesForward := link.TenantA() == tenantA && link.TenantB() == tenantB
		matchesReversed := link.TenantA() == tenantB && link.TenantB() == tenantA
		if matchesForward || matchesReversed {
			return link, true, nil
		}
	}
	return domain.TenantLink{}, false, nil
}

func (r fakeTenantLinkRepository) ByID(ctx context.Context, linkID string) (domain.TenantLink, error) {
	link, ok := r.store.tenantLinks[linkID]
	if !ok {
		return domain.TenantLink{}, domain.NewTenantLinkNotFound()
	}
	return link, nil
}

func (r fakeTenantLinkRepository) Revoke(ctx context.Context, linkID string) error {
	link, ok := r.store.tenantLinks[linkID]
	if !ok {
		return domain.NewTenantLinkNotFound()
	}
	revoked, err := domain.RevokeTenantLink(linkID, []domain.TenantLink{link})
	if err != nil {
		return err
	}
	r.store.tenantLinks[linkID] = revoked
	return nil
}

// fakeCounterpartyAliasRepository joined the other fakes as of step 02-03
// (inter-tenant-transfer). Keyed on the composite (tenant_id, alias)
// identity, mirroring the real table's own primary key (migration 02-01).
type fakeCounterpartyAliasRepository struct{ store *fakeStore }

var _ ports.CounterpartyAliasRepository = fakeCounterpartyAliasRepository{}

func counterpartyAliasKey(tenantID, alias string) string {
	return tenantID + "\x00" + alias
}

func (r fakeCounterpartyAliasRepository) Create(ctx context.Context, alias domain.CounterpartyAlias) error {
	key := counterpartyAliasKey(alias.TenantID(), alias.Alias())
	if _, exists := r.store.counterpartyAlias[key]; exists {
		return fmt.Errorf("fakeCounterpartyAliasRepository: alias %q already registered for tenant %q", alias.Alias(), alias.TenantID())
	}
	r.store.counterpartyAlias[key] = alias
	return nil
}

func (r fakeCounterpartyAliasRepository) ByTenantAndAlias(ctx context.Context, tenantID, alias string) (domain.CounterpartyAlias, bool, error) {
	got, ok := r.store.counterpartyAlias[counterpartyAliasKey(tenantID, alias)]
	return got, ok, nil
}

// fakeTransferStateRepository joined the other fakes as of step 02-03
// (inter-tenant-transfer). ClaimOne mirrors the real adapter's eligibility
// predicate (status pending/retrying, next_attempt_at already due); the
// concurrent-claim race itself is proven for real against PostgreSQL in
// internal/adapters/postgres (this fake runs single-threaded within one
// test, so there is no race to model here — mirrors this suite's own
// TEST PARADIGM note for the other transactional ports).
type fakeTransferStateRepository struct{ store *fakeStore }

var _ ports.TransferStateRepository = fakeTransferStateRepository{}

func (r fakeTransferStateRepository) Create(ctx context.Context, state ports.TransferState) error {
	if _, exists := r.store.transferStates[state.TransferID]; exists {
		return fmt.Errorf("fakeTransferStateRepository: transfer %q already exists", state.TransferID)
	}
	r.store.transferStates[state.TransferID] = state
	return nil
}

func (r fakeTransferStateRepository) Get(ctx context.Context, transferID string) (ports.TransferState, bool, error) {
	state, ok := r.store.transferStates[transferID]
	return state, ok, nil
}

func (r fakeTransferStateRepository) UpdateStatus(ctx context.Context, transferID string, status string, reason string) error {
	state, ok := r.store.transferStates[transferID]
	if !ok {
		return fmt.Errorf("fakeTransferStateRepository: unknown transfer %q", transferID)
	}
	state.Status = status
	state.Reason = reason
	r.store.transferStates[transferID] = state
	return nil
}

// AdvanceAfterLegOutcome mirrors the real adapter's own combined
// status/next_attempt_at write (step 02-05) — added alongside the real
// port's identical addition, mechanical parity only, no new assertion
// behavior.
func (r fakeTransferStateRepository) AdvanceAfterLegOutcome(ctx context.Context, transferID string, status string, nextAttemptAt time.Time, reason string) error {
	state, ok := r.store.transferStates[transferID]
	if !ok {
		return fmt.Errorf("fakeTransferStateRepository: unknown transfer %q", transferID)
	}
	state.Status = status
	state.NextAttemptAt = nextAttemptAt
	state.Reason = reason
	r.store.transferStates[transferID] = state
	return nil
}

func (r fakeTransferStateRepository) ClaimOne(ctx context.Context, transferID string, leaseDuration time.Duration) (ports.TransferState, bool, error) {
	state, ok := r.store.transferStates[transferID]
	if !ok {
		return ports.TransferState{}, false, nil
	}
	dueStatus := state.Status == "pending" || state.Status == "retrying"
	if !dueStatus || state.NextAttemptAt.After(time.Now().UTC()) {
		return ports.TransferState{}, false, nil
	}
	state.NextAttemptAt = time.Now().UTC().Add(leaseDuration)
	r.store.transferStates[transferID] = state
	return state, true, nil
}

func (r fakeTransferStateRepository) ClaimDue(ctx context.Context, now time.Time, batchLimit int) ([]string, error) {
	ids := make([]string, 0, len(r.store.transferStates))
	for id := range r.store.transferStates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return r.store.transferStates[ids[i]].NextAttemptAt.Before(r.store.transferStates[ids[j]].NextAttemptAt)
	})

	var due []string
	for _, id := range ids {
		state := r.store.transferStates[id]
		dueStatus := state.Status == "pending" || state.Status == "retrying"
		if dueStatus && !state.NextAttemptAt.After(now) {
			due = append(due, id)
		}
		if len(due) == batchLimit {
			break
		}
	}
	return due, nil
}
