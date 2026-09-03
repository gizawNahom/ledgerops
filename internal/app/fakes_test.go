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

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// fakeStore is the shared state a fakeUnitOfWork's repositories read and
// write. One fakeStore per test case — never shared across rapid.Check
// iterations — so before/after snapshots are never contaminated by a prior
// draw.
type fakeStore struct {
	accounts  map[string]domain.Account
	postings  map[string]domain.Posting
	entries   []domain.Entry
	claims    map[string]ports.Claim
	tenants   map[string]domain.Tenant
	committed bool
}

func newFakeStore(accounts ...domain.Account) *fakeStore {
	byID := make(map[string]domain.Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID()] = account
	}
	return &fakeStore{
		accounts: byID,
		postings: map[string]domain.Posting{},
		claims:   map[string]ports.Claim{},
		tenants:  map[string]domain.Tenant{},
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

func (r fakeAccountRepository) LockForUpdate(ctx context.Context, accountIDs []string) ([]domain.Account, error) {
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

func (r fakeAccountRepository) ApplyDeltas(ctx context.Context, deltas []domain.BalanceDelta) error {
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

func (r fakeAccountRepository) Create(ctx context.Context, account domain.Account) error {
	if _, exists := r.store.accounts[account.ID()]; exists {
		return fmt.Errorf("fakeAccountRepository: account %q already exists", account.ID())
	}
	r.store.accounts[account.ID()] = account
	return nil
}

func (r fakeAccountRepository) Get(ctx context.Context, accountID string) (domain.Account, error) {
	account, ok := r.store.accounts[accountID]
	if !ok {
		return domain.Account{}, domain.NewUnknownAccount(accountID)
	}
	return account, nil
}

// All enumerates every account, ordered by id — VerifyBooks' full-scan
// contract (D9) is what this fake exists for.
func (r fakeAccountRepository) All(ctx context.Context) ([]domain.Account, error) {
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

type fakeTransactionRepository struct{ store *fakeStore }

var _ ports.TransactionRepository = fakeTransactionRepository{}

func (r fakeTransactionRepository) Append(ctx context.Context, posting domain.Posting) error {
	if _, exists := r.store.postings[posting.Transaction.ID()]; exists {
		return fmt.Errorf("fakeTransactionRepository: transaction %q already recorded", posting.Transaction.ID())
	}
	r.store.postings[posting.Transaction.ID()] = posting
	r.store.entries = append(r.store.entries, posting.Entries...)
	return nil
}

func (r fakeTransactionRepository) Get(ctx context.Context, transactionID string) (domain.Posting, error) {
	posting, ok := r.store.postings[transactionID]
	if !ok {
		return domain.Posting{}, fmt.Errorf("fakeTransactionRepository: unknown transaction %q", transactionID)
	}
	return posting, nil
}

func (r fakeTransactionRepository) EntriesFor(ctx context.Context, accountID string) ([]domain.Entry, error) {
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
func (r fakeTransactionRepository) TrialBalance(ctx context.Context) (domain.Money, int, error) {
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
func (r fakeTransactionRepository) ComputedBalances(ctx context.Context) (map[string]domain.Money, error) {
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

func (r fakeTenantRepository) Create(ctx context.Context, tenant domain.Tenant) error {
	if _, exists := r.store.tenants[tenant.Name()]; exists {
		return fmt.Errorf("fakeTenantRepository: tenant name %q already exists", tenant.Name())
	}
	r.store.tenants[tenant.Name()] = tenant
	return nil
}
