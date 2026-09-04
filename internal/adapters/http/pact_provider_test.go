// pact_provider_test.go is the provider side of the console/ledgercore
// contract: it replays every interaction pinned in the consumer-driven pact
// file (web/console/pacts/console-ledgercore.json, produced by the console's
// consumer test in step 01-01) against the REAL driving adapter — the same
// chi router and handlers.go bodies NewRouter wires in production — over a
// real httptest.Server, following the acceptance suite's convention
// (tests/acceptance/ledgercore) of standing the production router up over a
// real socket rather than calling handler functions directly.
//
// The only fake here is the ports.Store: a real PostgreSQL round-trip is
// already proven by the acceptance suite and internal/adapters/postgres —
// this test's job is narrower: does the wire SHAPE this handler emits honour
// what the console pinned? A fake store, wired through the same
// app.Ledger/ports boundary the real postgres adapter satisfies, is the
// right weight for that question (Mandate 6 already covers the adapter's own
// real-I/O obligation elsewhere).
//
// Provider state setup for the verdict interaction ("the books are out of
// balance with one drifted account") reaches past the driving ports to
// mutate the fake store's stored balance directly — the one case this suite
// intentionally bypasses HTTP, for exactly the reason ledger_world.go's
// AttemptTamper/CorruptStoredBalance does: producing a state the application
// is structurally incapable of producing through its own front door.
package http_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// httpDoer is the sliver of *http.Client the state-setup helpers need —
// named so seedAccountWithOneEntry/seedOneDriftedAccount's signatures read as
// "calls the real front door", not "holds a concrete client type".
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// callJSON drives one request through the real front door carrying the
// platform-admin operator key -- the credential every /tenants provisioning
// call still requires (DDD-22 leaves that gate untouched).
func callJSON(ctx context.Context, client httpDoer, baseURL, method, path string, body any) error {
	return callJSONAs(ctx, client, baseURL, method, path, pactOperatorKey, "", body)
}

// callJSONAs is callJSON generalized over which
// bearer credential rides in the Authorization header -- POST /accounts and
// POST /transfers no longer accept the operator key at all as of step 02-03
// (DDD-23 Option C), so this test's own seed helpers below present a
// provisioned tenant's own key instead.
func callJSONAs(ctx context.Context, client httpDoer, baseURL, method, path, bearerKey, idempotencyKey string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding %s %s body: %w", method, path, err)
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("building %s %s request: %w", method, path, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+bearerKey)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		var problem map[string]any
		_ = json.NewDecoder(response.Body).Decode(&problem)
		return fmt.Errorf("%s %s: status %d, body %v", method, path, response.StatusCode, problem)
	}
	return nil
}

// provisionPactTenant provisions a tenant through the real front door (POST
// /tenants, still operator-key-gated -- unchanged by step 02-03) and returns
// its issued tenant_key, so a seed helper can present a real tenant
// credential to POST /accounts and POST /transfers instead of the operator
// key those routes stopped accepting.
func provisionPactTenant(ctx context.Context, client httpDoer, baseURL, name string) (string, error) {
	encoded, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return "", fmt.Errorf("encoding /tenants body: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/tenants", bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("building POST /tenants request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+pactOperatorKey)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("POST /tenants: %w", err)
	}
	defer response.Body.Close()

	var payload struct {
		TenantKey string `json:"tenant_key"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding POST /tenants response: %w", err)
	}
	if response.StatusCode != http.StatusCreated || payload.TenantKey == "" {
		return "", fmt.Errorf("POST /tenants for %q: status %d, no tenant_key issued", name, response.StatusCode)
	}
	return payload.TenantKey, nil
}

// pactTenantKeyResolver returns a ports.TenantKeyResolver over the fake
// store's provisioned tenants -- the in-process equivalent of
// postgres.NewTenantKeyResolver (which queries a real tenants table), sized
// for this test's fake store rather than a real database.
func pactTenantKeyResolver(store *pactFakeStore) ports.TenantKeyResolver {
	return func(ctx context.Context, credentialHash string) (string, bool, error) {
		for _, tenant := range store.tenants {
			if pactHashCredential(tenant.Credential()) == credentialHash {
				return tenant.TenantID(), true, nil
			}
		}
		return "", false, nil
	}
}

// pactHashCredential mirrors postgres.hashCredential's digest/encoding shape
// (sha256, hex-encoded) -- the same shape router.go's hashBearerToken
// produces from a presented bearer token, so the two sides of this fake
// resolver agree on what "the same credential" means.
func pactHashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

// pactOperatorKey MUST equal the literal Authorization value the consumer
// pinned ("Bearer a-valid-operator-key") — the pact's matching rule on that
// header is "type" (any string), but provider verification always REPLAYS
// the recorded request bytes, never lenience-adjusts them. The router's
// OperatorKey has to accept that exact literal or every interaction fails
// with 401 before ever reaching the wire-shape question this test exists to
// answer.
const pactOperatorKey = "a-valid-operator-key"

// TestPactProvider_HonorsConsoleContract replays the console's pinned
// interactions (GET /accounts/{id}/entries, GET /console/verdict) against
// the real handlers and fails if the real response shape has drifted from
// what the consumer's contract test recorded.
func TestPactProvider_HonorsConsoleContract(t *testing.T) {
	store := newPactFakeStore()

	handler := apphttp.NewRouter(apphttp.Deps{
		Store:             store,
		OperatorKey:       pactOperatorKey,
		Clock:             func() time.Time { return time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC) },
		IDGenerator:       sequentialTransactionIDs(),
		TenantKeyResolver: pactTenantKeyResolver(store),
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	pactFile, err := filepath.Abs(filepath.Join("..", "..", "..", "web", "console", "pacts", "console-ledgercore.json"))
	if err != nil {
		t.Fatalf("resolving pact file path: %v", err)
	}

	verifier := provider.NewVerifier()

	err = verifier.VerifyProvider(t, provider.VerifyRequest{
		Provider:        "ledgercore",
		ProviderBaseURL: server.URL,
		PactFiles:       []string{pactFile},
		StateHandlers: models.StateHandlers{
			"account alice-demo has at least one entry": func(setup bool, _ models.ProviderState) (models.ProviderStateResponse, error) {
				if !setup {
					return nil, nil
				}
				return nil, seedAccountWithOneEntry(context.Background(), server.Client(), server.URL, store)
			},
			"the books are out of balance with one drifted account": func(setup bool, _ models.ProviderState) (models.ProviderStateResponse, error) {
				if !setup {
					return nil, nil
				}
				return nil, seedOneDriftedAccount(context.Background(), server.Client(), server.URL, store)
			},
		},
	})
	if err != nil {
		t.Fatalf("pact provider verification: %v", err)
	}
}

// seedAccountWithOneEntry provisions a tenant, then opens alice-demo and a
// funding account under that tenant's own key through the real driving
// ports (POST /accounts, POST /transfers -- tenant-key-only as of step
// 02-03, DDD-23 Option C) and settles one transfer into alice-demo, so GET
// /accounts/alice-demo/entries has at least one row to answer with.
func seedAccountWithOneEntry(ctx context.Context, client httpDoer, baseURL string, store *pactFakeStore) error {
	tenantKey, err := provisionPactTenant(ctx, client, baseURL, "pact-seed-alice-demo")
	if err != nil {
		return err
	}
	if err := callJSONAs(ctx, client, baseURL, "POST", "/accounts", tenantKey, "",
		map[string]any{"account_id": "treasury-alice", "type": "system"}); err != nil {
		return err
	}
	if err := callJSONAs(ctx, client, baseURL, "POST", "/accounts", tenantKey, "",
		map[string]any{"account_id": "alice-demo", "type": "wallet"}); err != nil {
		return err
	}
	if err := callJSONAs(ctx, client, baseURL, "POST", "/transfers", tenantKey, "seed-alice-demo-entry",
		map[string]any{"from": "treasury-alice", "to": "alice-demo", "amount": "12.34"}); err != nil {
		return err
	}
	return nil
}

// seedOneDriftedAccount opens an account through the real front door, then
// reaches past the driving ports to rewrite its stored balance directly —
// the application has no legal path to produce a stored/computed
// disagreement (I3 holds by construction through every driving port), so
// this is deliberately the one place this test bypasses HTTP, mirroring
// ledger_world.go's AttemptTamper/CorruptStoredBalance in the acceptance
// suite.
func seedOneDriftedAccount(ctx context.Context, client httpDoer, baseURL string, store *pactFakeStore) error {
	const driftedAccountID = "acct-42"
	tenantKey, err := provisionPactTenant(ctx, client, baseURL, "pact-seed-drifted")
	if err != nil {
		return err
	}
	if err := callJSONAs(ctx, client, baseURL, "POST", "/accounts", tenantKey, "",
		map[string]any{"account_id": driftedAccountID, "type": "wallet"}); err != nil {
		return err
	}
	drifted, err := domain.NewMoney(500, "USD") // 5.00 stored, 0.00 computed (no entries) — one drifted row
	if err != nil {
		return err
	}
	return store.corruptStoredBalance(driftedAccountID, drifted)
}

// sequentialTransactionIDs hands out deterministic, distinct transaction ids
// — PostTransfer calls it once per non-replayed posting.
func sequentialTransactionIDs() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("pact-txn-%d", next)
	}
}

// --- pactFakeStore: the fake ports.Store this provider test wires in place
// of a real database. Shaped like internal/app/fakes_test.go's fakeStore
// (same port contracts, same input-validation discipline), but declared
// locally: that file lives in package app_test in a different directory and
// is not importable from here.

type pactFakeStore struct {
	accounts map[string]domain.Account
	postings map[string]domain.Posting
	entries  []domain.Entry
	claims   map[string]ports.Claim
	tenants  map[string]domain.Tenant
}

func newPactFakeStore() *pactFakeStore {
	return &pactFakeStore{
		accounts: map[string]domain.Account{},
		postings: map[string]domain.Posting{},
		claims:   map[string]ports.Claim{},
		tenants:  map[string]domain.Tenant{},
	}
}

// corruptStoredBalance overwrites one account's stored balance in place,
// leaving its entries (and therefore its computed balance) untouched — the
// direct mutation that manufactures drift, see seedOneDriftedAccount.
func (s *pactFakeStore) corruptStoredBalance(accountID string, balance domain.Money) error {
	account, ok := s.accounts[accountID]
	if !ok {
		return fmt.Errorf("pactFakeStore: unknown account %q", accountID)
	}
	rebuilt, err := domain.NewAccount(account.TenantID(), account.ID(), account.Kind(), balance)
	if err != nil {
		return err
	}
	s.accounts[accountID] = rebuilt
	return nil
}

func (s *pactFakeStore) Begin(ctx context.Context) (ports.UnitOfWork, error) {
	return &pactFakeUnitOfWork{store: s}, nil
}

func (s *pactFakeStore) Close() error { return nil }

var _ ports.Store = (*pactFakeStore)(nil)

type pactFakeUnitOfWork struct{ store *pactFakeStore }

func (u *pactFakeUnitOfWork) Accounts() ports.AccountRepository {
	return pactFakeAccountRepository{u.store}
}
func (u *pactFakeUnitOfWork) Transactions() ports.TransactionRepository {
	return pactFakeTransactionRepository{u.store}
}
func (u *pactFakeUnitOfWork) Idempotency() ports.IdempotencyStore {
	return pactFakeIdempotencyStore{u.store}
}
func (u *pactFakeUnitOfWork) Tenants() ports.TenantRepository {
	return pactFakeTenantRepository{u.store}
}
func (u *pactFakeUnitOfWork) Commit(ctx context.Context) error   { return nil }
func (u *pactFakeUnitOfWork) Rollback(ctx context.Context) error { return nil }

var _ ports.UnitOfWork = (*pactFakeUnitOfWork)(nil)

type pactFakeAccountRepository struct{ store *pactFakeStore }

var _ ports.AccountRepository = pactFakeAccountRepository{}

func (r pactFakeAccountRepository) LockForUpdate(ctx context.Context, tenantID string, accountIDs []string) ([]domain.Account, error) {
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

func (r pactFakeAccountRepository) ApplyDeltas(ctx context.Context, tenantID string, deltas []domain.BalanceDelta) error {
	for _, delta := range deltas {
		account, ok := r.store.accounts[delta.AccountID]
		if !ok {
			return fmt.Errorf("pactFakeAccountRepository: unknown account %q", delta.AccountID)
		}
		updated, err := account.Apply(delta.Delta)
		if err != nil {
			return err
		}
		r.store.accounts[delta.AccountID] = updated
	}
	return nil
}

func (r pactFakeAccountRepository) Create(ctx context.Context, tenantID string, account domain.Account) error {
	if _, exists := r.store.accounts[account.ID()]; exists {
		return fmt.Errorf("pactFakeAccountRepository: account %q already exists", account.ID())
	}
	r.store.accounts[account.ID()] = account
	return nil
}

func (r pactFakeAccountRepository) Get(ctx context.Context, tenantID string, accountID string) (domain.Account, error) {
	account, ok := r.store.accounts[accountID]
	if !ok {
		return domain.Account{}, domain.NewUnknownAccount(accountID)
	}
	return account, nil
}

func (r pactFakeAccountRepository) ExistsAnyTenant(ctx context.Context, accountID string) (bool, error) {
	_, ok := r.store.accounts[accountID]
	return ok, nil
}

func (r pactFakeAccountRepository) All(ctx context.Context, tenantID string) ([]domain.Account, error) {
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

type pactFakeTransactionRepository struct{ store *pactFakeStore }

var _ ports.TransactionRepository = pactFakeTransactionRepository{}

func (r pactFakeTransactionRepository) Append(ctx context.Context, tenantID string, posting domain.Posting) error {
	if _, exists := r.store.postings[posting.Transaction.ID()]; exists {
		return fmt.Errorf("pactFakeTransactionRepository: transaction %q already recorded", posting.Transaction.ID())
	}
	r.store.postings[posting.Transaction.ID()] = posting
	r.store.entries = append(r.store.entries, posting.Entries...)
	return nil
}

func (r pactFakeTransactionRepository) Get(ctx context.Context, tenantID string, transactionID string) (domain.Posting, error) {
	posting, ok := r.store.postings[transactionID]
	if !ok {
		return domain.Posting{}, fmt.Errorf("pactFakeTransactionRepository: unknown transaction %q", transactionID)
	}
	return posting, nil
}

func (r pactFakeTransactionRepository) EntriesFor(ctx context.Context, scope ports.TenantScope, accountID string) ([]domain.Entry, error) {
	var entries []domain.Entry
	for _, entry := range r.store.entries {
		if entry.AccountID() == accountID {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (r pactFakeTransactionRepository) TrialBalance(ctx context.Context, scope ports.TenantScope) (domain.Money, int, error) {
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

func (r pactFakeTransactionRepository) ComputedBalances(ctx context.Context, scope ports.TenantScope) (map[string]domain.Money, error) {
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

type pactFakeIdempotencyStore struct{ store *pactFakeStore }

var _ ports.IdempotencyStore = pactFakeIdempotencyStore{}

func (r pactFakeIdempotencyStore) Claim(ctx context.Context, key, fingerprint, transactionID string) (ports.Claim, error) {
	if existing, ok := r.store.claims[key]; ok {
		return ports.Claim{}, fmt.Errorf("pactFakeIdempotencyStore: key %q already claimed by transaction %q", key, existing.TransactionID)
	}
	claim := ports.Claim{Key: key, Fingerprint: fingerprint, TransactionID: transactionID}
	r.store.claims[key] = claim
	return claim, nil
}

func (r pactFakeIdempotencyStore) Lookup(ctx context.Context, key string) (ports.Claim, bool, error) {
	claim, ok := r.store.claims[key]
	return claim, ok, nil
}

// pactFakeTenantRepository joined the other pact fakes as of step 01-03
// (multitenancy) — no pact interaction in this suite exercises
// ProvisionTenant today, but ports.UnitOfWork now requires Tenants(), so this
// keeps pactFakeUnitOfWork satisfying the interface with the same
// input-validation discipline as its siblings.
type pactFakeTenantRepository struct{ store *pactFakeStore }

var _ ports.TenantRepository = pactFakeTenantRepository{}

func (r pactFakeTenantRepository) ByName(ctx context.Context, name string) (domain.Tenant, error) {
	tenant, ok := r.store.tenants[name]
	if !ok {
		return domain.Tenant{}, domain.NewTenantNotFound(name)
	}
	return tenant, nil
}

// ByID joined ByName as of step 03-01 — GET /health/trial-balance's
// tenant-scoped existence check now requires it on every ports.TenantRepository,
// this fake included, keeping pactFakeUnitOfWork satisfying the interface.
func (r pactFakeTenantRepository) ByID(ctx context.Context, tenantID string) (domain.Tenant, error) {
	for _, tenant := range r.store.tenants {
		if tenant.TenantID() == tenantID {
			return tenant, nil
		}
	}
	return domain.Tenant{}, domain.NewTenantNotFound(tenantID)
}

func (r pactFakeTenantRepository) Create(ctx context.Context, tenant domain.Tenant) error {
	if _, exists := r.store.tenants[tenant.Name()]; exists {
		return fmt.Errorf("pactFakeTenantRepository: tenant name %q already exists", tenant.Name())
	}
	r.store.tenants[tenant.Name()] = tenant
	return nil
}
