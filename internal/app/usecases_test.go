// Package app_test exercises the shell through its driving ports — the
// exported methods of *app.Ledger — over fake repositories (permitted at this
// layer, see fakes_test.go). The real repositories are exercised for real
// against PostgreSQL via Testcontainers in internal/adapters/postgres
// (Mandate 6); this suite proves PostTransfer's ORCHESTRATION: lock, decide,
// write, all inside one unit of work.
//
// Property-based by default (pgregory.net/rapid) with state-delta matchers
// (tests/common/statedelta), per step 01-03's mandated TEST PARADIGM.
package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
	"ledgerops/tests/common/statedelta"
)

// testTenantID mirrors app.legacyTenantID (unexported, so this suite
// names its own copy) -- the interim single-tenant identity every
// app.Ledger write-path call resolves to until step 02-04 wires real
// per-request tenant extraction. Accounts seeded directly via
// domain.NewAccount for this suite use the same constant so a seeded
// snapshot's TenantID() matches what Ledger itself threads through to
// domain.Post's I8 cross-check.
const testTenantID = "tnt_legacy_seed"

func fixedClock() time.Time {
	return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
}

func sequentialIDs() ports.IDGenerator {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("txn-%d", n)
	}
}

func captureUniverse(store *fakeStore, from, to string) statedelta.Snapshot {
	return statedelta.Snapshot{
		"account.from.balance": store.accounts[from].Balance().MinorUnits(),
		"account.to.balance":   store.accounts[to].Balance().MinorUnits(),
		"ledger.entry_count":   len(store.entries),
		"idempotency.claimed":  len(store.claims) > 0,
	}
}

var universe = []string{
	"account.from.balance",
	"account.to.balance",
	"ledger.entry_count",
	"idempotency.claimed",
}

// TestProperty_PostTransfer_MovesValueAndClaimsTheKeyAtomically covers the
// bounded-change contract for PostTransfer's success path (step 01-03
// implementation_notes): the universe is the touched accounts' balances, the
// new transaction's entries, and the idempotency record — everything else
// declared must be unchanged, and here nothing else is declared, so the
// assertion is exact.
func TestProperty_PostTransfer_MovesValueAndClaimsTheKeyAtomically(t *testing.T) {
	// AssertStateDelta needs testing.TB, which *rapid.T does not implement
	// (TB is a sealed interface) — draws happen on rt, assertions on the
	// outer t, per rapid's own recommended pattern for interop with
	// TB-typed helpers.
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(1, 1_000_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(1, fromStartMinor).Draw(rt, "amount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount(testTenantID, "from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount(testTenantID, "to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		before := captureUniverse(store, "from", "to")
		result, err := ledger.PostTransfer(context.Background(), app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
			TenantID:       testTenantID,
		})
		if err != nil {
			rt.Fatalf("unexpected refusal moving %d of %d available: %v", amountMinor, fromStartMinor, err)
		}
		after := captureUniverse(store, "from", "to")

		statedelta.AssertStateDelta(t, before, after, universe, map[string]statedelta.Predicate{
			"account.from.balance": statedelta.SetTo(fromStartMinor - amountMinor),
			"account.to.balance":   statedelta.SetTo(amountMinor),
			"ledger.entry_count":   statedelta.SetTo(2),
			"idempotency.claimed":  statedelta.SetTo(true),
		})

		if result.Replayed {
			rt.Fatalf("a first-time posting must not report Replayed")
		}
		if !store.committed {
			rt.Fatalf("PostTransfer succeeded without committing the unit of work")
		}
	})
}

// TestProperty_PostTransfer_RefusalLeavesEveryUniverseSlotUnchanged covers the
// unbounded-preservation side of the same contract: when domain.Post refuses
// a movement (insufficient funds), nothing in the universe changes — the
// Write step never runs, and the deferred Rollback is what makes that true
// even if it did.
func TestProperty_PostTransfer_RefusalLeavesEveryUniverseSlotUnchanged(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(0, 1_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(fromStartMinor+1, fromStartMinor+1_000_000).Draw(rt, "amount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount(testTenantID, "from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount(testTenantID, "to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		before := captureUniverse(store, "from", "to")
		_, err = ledger.PostTransfer(context.Background(), app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
			TenantID:       testTenantID,
		})
		after := captureUniverse(store, "from", "to")

		var violation domain.Violation
		if err == nil {
			rt.Fatalf("expected insufficient_funds moving %d of %d available, got success", amountMinor, fromStartMinor)
		}
		if !errors.As(err, &violation) || violation.Kind() != domain.InsufficientFunds {
			rt.Fatalf("expected insufficient_funds, got %v", err)
		}

		// No expected map: every universe slot defaults to Unchanged().
		statedelta.AssertStateDelta(t, before, after, universe, map[string]statedelta.Predicate{})
	})
}

// TestProperty_PostTransfer_RepeatedApplicationEqualsOneApplication covers
// this step's TEST PARADIGM obligation: for any request and any repeat count
// N >= 1, submitting the SAME idempotency key and fingerprint N times against
// the same fake ports leaves the final state identical to what a single
// application produces. Call 1 is the real write; calls 2..N must all be
// replays — Replayed: true, the same TransactionID, and the universe
// unchanged from the state call 1 left behind.
func TestProperty_PostTransfer_RepeatedApplicationEqualsOneApplication(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(1, 1_000_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(1, fromStartMinor).Draw(rt, "amount")
		repeatCount := rapid.IntRange(1, 5).Draw(rt, "repeatCount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount(testTenantID, "from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount(testTenantID, "to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		request := app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
			TenantID:       testTenantID,
		}

		first, err := ledger.PostTransfer(context.Background(), request)
		if err != nil {
			rt.Fatalf("unexpected refusal on first application: %v", err)
		}
		if first.Replayed {
			rt.Fatalf("a first-time posting must not report Replayed")
		}

		afterFirst := captureUniverse(store, "from", "to")

		for i := 2; i <= repeatCount; i++ {
			repeat, err := ledger.PostTransfer(context.Background(), request)
			if err != nil {
				rt.Fatalf("unexpected refusal on repeat %d: %v", i, err)
			}
			if !repeat.Replayed {
				rt.Fatalf("repeat %d must report Replayed, got a fresh write", i)
			}
			if repeat.Posting.Transaction.ID() != first.Posting.Transaction.ID() {
				rt.Fatalf("repeat %d TransactionID = %q, want %q (same as first application)",
					i, repeat.Posting.Transaction.ID(), first.Posting.Transaction.ID())
			}

			afterRepeat := captureUniverse(store, "from", "to")
			statedelta.AssertStateDelta(t, afterFirst, afterRepeat, universe, map[string]statedelta.Predicate{})
		}
	})
}

// TestProperty_CreateAccount_OpensAtZeroBalance covers CreateAccount's
// contract: a newly opened account of either kind starts at zero, through the
// real AccountRepository.Create call (here, the fake honouring the same
// duplicate-id refusal the real one does).
func TestProperty_CreateAccount_OpensAtZeroBalance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		accountID := rapid.StringMatching(`[a-z][a-z0-9_]{2,12}`).Draw(t, "accountID")
		kind := rapid.SampledFrom([]domain.AccountKind{domain.Wallet, domain.System}).Draw(t, "kind")

		store := newFakeStore()
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		if err := ledger.CreateAccount(context.Background(), testTenantID, accountID, kind); err != nil {
			t.Fatalf("unexpected error opening account %q: %v", accountID, err)
		}

		account, ok := store.accounts[accountID]
		if !ok {
			t.Fatalf("account %q was not persisted", accountID)
		}
		if account.Kind() != kind {
			t.Fatalf("Kind() = %v, want %v", account.Kind(), kind)
		}
		if account.Balance().MinorUnits() != 0 {
			t.Fatalf("Balance().MinorUnits() = %d, want 0", account.Balance().MinorUnits())
		}
		if !store.committed {
			t.Fatalf("CreateAccount succeeded without committing the unit of work")
		}
	})
}

// TestGetBalance_ReadsStoredBalance covers GetBalance's contract: it reads
// through the repository and performs no I/O of its own beyond that read.
//
// bypass: single example, not a property — GetBalance is a pure pass-through
// read with no side effect to declare a universe over (nw-tdd-methodology
// exempt category "pure-function tests with single output and no side
// effects").
func TestGetBalance_ReadsStoredBalance(t *testing.T) {
	balance, err := domain.NewMoney(500, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	account, err := domain.NewAccount(testTenantID, "wallet-1", domain.Wallet, balance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	store := newFakeStore(account)
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	got, err := ledger.GetBalance(context.Background(), testTenantID, "wallet-1")
	if err != nil {
		t.Fatalf("unexpected error reading balance: %v", err)
	}
	if got.Balance().MinorUnits() != 500 {
		t.Fatalf("Balance().MinorUnits() = %d, want 500", got.Balance().MinorUnits())
	}
}

// TestGetBalance_UnknownAccountIsRefused covers the negative path: reading a
// balance for an account that was never opened is a refusal, not a panic or
// a zero value that could be mistaken for a real one.
func TestGetBalance_UnknownAccountIsRefused(t *testing.T) {
	store := newFakeStore()
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	_, err := ledger.GetBalance(context.Background(), testTenantID, "ghost")

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found, got %v", err)
	}
}

// TestProperty_VerifyBooks_AgreeingBalancesAreReportedHealthy covers
// VerifyBooks' healthy-path invariant (step 06-01 TEST PARADIGM note): for
// any set of accounts whose stored balances already equal what their own
// entries sum to, the verdict is Balanced with no drifted rows — regardless
// of how many accounts there are or what their balances happen to be.
func TestProperty_VerifyBooks_AgreeingBalancesAreReportedHealthy(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		accountCount := rapid.IntRange(0, 6).Draw(rt, "accountCount")

		accounts := make([]domain.Account, 0, accountCount)
		entries := make([]domain.Entry, 0, accountCount)
		for i := 0; i < accountCount; i++ {
			accountID := fmt.Sprintf("acct-%d", i)
			balanceMinor := rapid.Int64Range(0, 1_000_000).Draw(rt, fmt.Sprintf("balance-%d", i))

			balance, err := domain.NewMoney(balanceMinor, "USD")
			if err != nil {
				t.Fatalf("NewMoney rejected a known currency: %v", err)
			}
			account, err := domain.NewAccount(testTenantID, accountID, domain.Wallet, balance)
			if err != nil {
				t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
			}
			accounts = append(accounts, account)

			// One entry per account whose amount equals the stored balance is
			// the smallest fixture that makes ComputedBalances agree with
			// Get — the property does not care HOW an account arrived at its
			// balance, only that the two independent representations (I3)
			// currently agree.
			entry, err := domain.NewEntry(fmt.Sprintf("txn-seed-%d", i), accountID, "seed", balance, fixedClock(), 1)
			if err != nil {
				t.Fatalf("NewEntry rejected a well-formed seed entry: %v", err)
			}
			entries = append(entries, entry)
		}

		store := newFakeStore(accounts...)
		store.entries = entries
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		report, err := ledger.VerifyBooks(context.Background(), ports.Unscoped())
		if err != nil {
			t.Fatalf("unexpected error verifying the books: %v", err)
		}
		if !report.Balanced {
			t.Fatalf("Balanced = false for agreeing balances, Drifted = %+v", report.Drifted)
		}
		if len(report.Drifted) != 0 {
			t.Fatalf("Drifted = %+v, want empty for agreeing balances", report.Drifted)
		}
		if report.EntryCount != len(entries) {
			t.Fatalf("EntryCount = %d, want %d", report.EntryCount, len(entries))
		}
	})
}

// TestProperty_VerifyBooks_TenantScopedCallThreadsTheSameScopeToEveryRead
// covers step 03-01's wiring contract: for any provisioned tenant id, a
// ScopedToTenant(id) call must narrow EVERY read VerifyBooks performs —
// Accounts().All and TrialBalance/ComputedBalances alike — to that same
// tenant, never widening any of them back to legacyTenantID or Unscoped().
// This is what stops the use case accidentally re-broadening a scoped query
// even though the repository layer (step 02-02) already enforces isolation
// underneath it — see usecases.go's VerifyBooks doc comment.
func TestProperty_VerifyBooks_TenantScopedCallThreadsTheSameScopeToEveryRead(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tenantID := "tnt_" + rapid.StringMatching(`[a-z][a-z0-9]{2,10}`).Draw(rt, "tenantID")

		store := newFakeStore()
		tenant, err := domain.NewTenant(tenantID, "tenant-"+tenantID, "tk_unused")
		if err != nil {
			t.Fatalf("NewTenant rejected a well-formed id: %v", err)
		}
		store.tenants[tenant.Name()] = tenant
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		_, err = ledger.VerifyBooks(context.Background(), ports.ScopedToTenant(tenantID))
		if err != nil {
			rt.Fatalf("unexpected refusal verifying a provisioned tenant's books: %v", err)
		}

		if store.lastAccountsAllTenantID != tenantID {
			rt.Fatalf("Accounts().All tenantID = %q, want %q", store.lastAccountsAllTenantID, tenantID)
		}
		gotTenantID, scoped := store.lastScope.Resolve()
		if !scoped || gotTenantID != tenantID {
			rt.Fatalf("TrialBalance/ComputedBalances scope = (%q, %v), want (%q, true)", gotTenantID, scoped, tenantID)
		}
	})
}

// TestVerifyBooks_UnscopedCallStaysPlatformWide is the regression assertion
// step 03-01's implementation_notes requires (DoD item 3a): an Unscoped()
// call still reads legacyTenantID's own accounts and an unscoped
// TrialBalance/ComputedBalances scope, exactly as it did before this step —
// adding the tenant-scoped branch must not perturb the pre-existing path.
func TestVerifyBooks_UnscopedCallStaysPlatformWide(t *testing.T) {
	store := newFakeStore()
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	if _, err := ledger.VerifyBooks(context.Background(), ports.Unscoped()); err != nil {
		t.Fatalf("unexpected error on an unscoped call: %v", err)
	}

	if store.lastAccountsAllTenantID != testTenantID {
		t.Fatalf("Accounts().All tenantID = %q, want legacyTenantID %q", store.lastAccountsAllTenantID, testTenantID)
	}
	if _, scoped := store.lastScope.Resolve(); scoped {
		t.Fatalf("TrialBalance/ComputedBalances scope was scoped, want Unscoped()")
	}
}

// TestVerifyBooks_UnprovisionedTenantIsRefusedBeforeAnyScan covers the
// tenant_not_found refusal DDD-25 requires: a tenant_id nobody ever
// provisioned must be refused BEFORE TrialBalance or ComputedBalances ever
// runs (design context, "check tenant existence first ... proceed to the
// scan only if it exists"). scanAttempted asserts absence of computation,
// not merely the error's shape.
func TestVerifyBooks_UnprovisionedTenantIsRefusedBeforeAnyScan(t *testing.T) {
	store := newFakeStore()
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	_, err := ledger.VerifyBooks(context.Background(), ports.ScopedToTenant("tnt_never-provisioned"))

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.TenantNotFound {
		t.Fatalf("expected tenant_not_found, got %v", err)
	}
	if violation.Tenant() != "tnt_never-provisioned" {
		t.Fatalf("Tenant() = %q, want %q", violation.Tenant(), "tnt_never-provisioned")
	}
	if store.scanAttempted {
		t.Fatalf("scanAttempted = true, want false — the scan must never run for an unprovisioned tenant")
	}
}

// captureProvisioningUniverse snapshots the three collections
// ProvisionTenant's bounded-change contract (step 01-03) is scoped over:
// tenants (the one collection allowed to change), accounts, and
// transactions (entries) — both of which must stay untouched by
// provisioning a tenant.
func captureProvisioningUniverse(store *fakeStore) statedelta.Snapshot {
	return statedelta.Snapshot{
		"tenants.count":      len(store.tenants),
		"accounts.count":     len(store.accounts),
		"transactions.count": len(store.entries),
	}
}

var provisioningUniverse = []string{
	"tenants.count",
	"accounts.count",
	"transactions.count",
}

// TestProperty_ProvisionTenant_BoundedChangeAddsExactlyOneTenant covers the
// contract-shape (implementation_notes, step 01-03): provisioning a tenant is
// bounded-change over the Tenant collection only — exactly one new tenant
// row, and the accounts/transactions collections are untouched
// (after.accounts == before.accounts, after.transactions ==
// before.transactions). The minted tenant_id/tenant_key carry the tnt_/tk_
// prefixes (credential format, brief.md § Multitenancy), and the plaintext
// tenant_key is returned exactly once in ProvisionedTenant.
func TestProperty_ProvisionTenant_BoundedChangeAddsExactlyOneTenant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		name := rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`).Draw(rt, "name")

		store := newFakeStore()
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		before := captureProvisioningUniverse(store)
		provisioned, err := ledger.ProvisionTenant(context.Background(), name)
		if err != nil {
			rt.Fatalf("unexpected refusal provisioning tenant %q: %v", name, err)
		}
		after := captureProvisioningUniverse(store)

		statedelta.AssertStateDelta(t, before, after, provisioningUniverse, map[string]statedelta.Predicate{
			"tenants.count": statedelta.SetTo(1),
		})

		if !strings.HasPrefix(provisioned.TenantID, "tnt_") {
			rt.Fatalf("TenantID = %q, want tnt_ prefix", provisioned.TenantID)
		}
		if !strings.HasPrefix(provisioned.TenantKey, "tk_") {
			rt.Fatalf("TenantKey = %q, want tk_ prefix", provisioned.TenantKey)
		}
		if provisioned.Name != name {
			rt.Fatalf("Name = %q, want %q", provisioned.Name, name)
		}
		if !store.committed {
			rt.Fatalf("ProvisionTenant succeeded without committing the unit of work")
		}

		stored, ok := store.tenants[name]
		if !ok {
			rt.Fatalf("tenant %q was not persisted", name)
		}
		if stored.TenantID() != provisioned.TenantID {
			rt.Fatalf("persisted TenantID() = %q, want %q", stored.TenantID(), provisioned.TenantID)
		}
	})
}

// TestProvisionTenant_DuplicateNameRefusedWithNoRowWritten covers this step's
// acceptance criterion verbatim: a duplicate tenant name is refused via
// domain's tenant_already_exists before any row is written. The universe
// (tenants included) must show zero change on the refusal path — the second
// call's Write step never runs.
//
// bypass: single example, not a property — two fixed calls with the same
// name is the whole scenario; a generated name changes nothing about what is
// being proven (nw-tdd-methodology exempt category does not literally cover
// this, but declaring the universe via AssertStateDelta below keeps the
// state-delta discipline rather than a bare post-state assert).
func TestProvisionTenant_DuplicateNameRefusedWithNoRowWritten(t *testing.T) {
	store := newFakeStore()
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())
	ctx := context.Background()

	if _, err := ledger.ProvisionTenant(ctx, "acme"); err != nil {
		t.Fatalf("unexpected error on first provisioning: %v", err)
	}

	before := captureProvisioningUniverse(store)
	_, err := ledger.ProvisionTenant(ctx, "acme")
	after := captureProvisioningUniverse(store)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.TenantAlreadyExists {
		t.Fatalf("expected tenant_already_exists, got %v", err)
	}

	// No expected map: every universe slot, tenants.count included, defaults
	// to Unchanged() — no row was written on the refusal path.
	statedelta.AssertStateDelta(t, before, after, provisioningUniverse, map[string]statedelta.Predicate{})
}
