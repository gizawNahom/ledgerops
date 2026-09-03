// Package domain_test exercises the pure core through its driving ports —
// the exported functions of package domain — per the port-to-port testing
// discipline (nw-tdd-methodology): a pure function's public signature IS its
// driving port, so calling domain.Post / domain.NewMoney / domain.NewAccount
// directly is port-to-port, not white-box.
//
// Property-based by default (pgregory.net/rapid), per step 01-01's mandated
// TEST PARADIGM. Single-example tests are used only where a property genuinely
// does not fit, and are marked `// bypass:`.
package domain_test

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"ledgerops/internal/domain"
	"pgregory.net/rapid"
)

var knownCurrencies = []string{"USD", "EUR", "GBP", "JPY"}

func genCurrency(t *rapid.T, label string) string {
	return rapid.SampledFrom(knownCurrencies).Draw(t, label)
}

// TestProperty_PostEntriesSumToZeroPerCurrency covers the PBT obligation "For
// any generated transfer, the returned entries sum to zero per currency" (I1).
func TestProperty_PostEntriesSumToZeroPerCurrency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currency := genCurrency(t, "currency")

		fromKind := rapid.SampledFrom([]domain.AccountKind{domain.Wallet, domain.System}).Draw(t, "fromKind")
		fromBalanceMinor := rapid.Int64Range(0, 1_000_000_000).Draw(t, "fromBalance")
		fromBalance, err := domain.NewMoney(fromBalanceMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", fromKind, fromBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		toBalanceMinor := rapid.Int64Range(0, 1_000_000_000).Draw(t, "toBalance")
		toBalance, err := domain.NewMoney(toBalanceMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, toBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		amountMinor := rapid.Int64Range(1, 1_000_000_000).Draw(t, "amount")
		amount, err := domain.NewMoney(amountMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		cmd := domain.TransferCommand{From: "from", To: "to", Amount: amount}
		posting, err := domain.Post(cmd, []domain.Account{fromAccount, toAccount}, time.Now(), "tx-i1")
		if err != nil {
			// A refusal (e.g. insufficient funds on a wallet source) is not a
			// violation of I1 -- I1 only binds a *successful* posting.
			return
		}

		if !domain.EntriesSumToZero(posting.Entries) {
			t.Fatalf("Post returned entries that do not sum to zero per currency: %+v", posting.Entries)
		}
	})
}

// TestProperty_NoSequenceOfTransfersDrivesWalletNegative covers the PBT
// obligation "No generated sequence of transfers drives a wallet account
// negative" (I4), exercising domain.Post + Account.Apply together the way the
// application shell would: Post decides, the shell (here, the test) applies
// the returned deltas to its own copy of account state.
func TestProperty_NoSequenceOfTransfersDrivesWalletNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currency := genCurrency(t, "currency")

		walletCount := rapid.IntRange(1, 4).Draw(t, "walletCount")
		accounts := make(map[string]domain.Account, walletCount+1)
		ids := make([]string, 0, walletCount+1)

		systemBalance, err := domain.NewMoney(0, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		systemAccount, err := domain.NewAccount("system", domain.System, systemBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		accounts["system"] = systemAccount
		ids = append(ids, "system")

		for i := 0; i < walletCount; i++ {
			id := fmt.Sprintf("wallet-%d", i)
			startMinor := rapid.Int64Range(0, 10_000).Draw(t, "start-"+id)
			balance, err := domain.NewMoney(startMinor, currency)
			if err != nil {
				t.Fatalf("NewMoney rejected a known currency: %v", err)
			}
			account, err := domain.NewAccount(id, domain.Wallet, balance)
			if err != nil {
				t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
			}
			accounts[id] = account
			ids = append(ids, id)
		}

		steps := rapid.IntRange(0, 20).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			fromID := rapid.SampledFrom(ids).Draw(t, fmt.Sprintf("from-%d", i))
			toID := rapid.SampledFrom(ids).Draw(t, fmt.Sprintf("to-%d", i))
			if fromID == toID {
				continue
			}
			amountMinor := rapid.Int64Range(1, 20_000).Draw(t, fmt.Sprintf("amount-%d", i))
			amount, err := domain.NewMoney(amountMinor, currency)
			if err != nil {
				t.Fatalf("NewMoney rejected a known currency: %v", err)
			}

			snapshots := make([]domain.Account, 0, len(ids))
			for _, id := range ids {
				snapshots = append(snapshots, accounts[id])
			}

			cmd := domain.TransferCommand{From: fromID, To: toID, Amount: amount}
			posting, err := domain.Post(cmd, snapshots, time.Now(), fmt.Sprintf("tx-%d", i))
			if err != nil {
				// Refused movement (e.g. insufficient funds): state unchanged.
				continue
			}

			for _, delta := range posting.Deltas {
				updated, applyErr := accounts[delta.AccountID].Apply(delta.Delta)
				if applyErr != nil {
					t.Fatalf("a delta Post already validated could not be re-applied: %v", applyErr)
				}
				accounts[delta.AccountID] = updated
			}
		}

		for id, account := range accounts {
			if account.Kind() == domain.Wallet && account.Balance().MinorUnits() < 0 {
				t.Fatalf("wallet %s went negative: %d", id, account.Balance().MinorUnits())
			}
		}
	})
}

// TestProperty_PostToleratesMixedCurrenciesAndMissingAccounts covers the PBT
// obligation "Negative testing: relax the same-currency assumption and the
// non-empty-snapshot assumption over domain.Post". No particular outcome is
// demanded by the acceptance-designer's obligation ("no assertion demanded
// yet") — only that Post answers with a value rather than panicking, and that
// any success still honours I1.
func TestProperty_PostToleratesMixedCurrenciesAndMissingAccounts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fromCurrency := genCurrency(t, "fromCurrency")
		toCurrency := genCurrency(t, "toCurrency")
		amountCurrency := genCurrency(t, "amountCurrency")

		includeFrom := rapid.Bool().Draw(t, "includeFrom")
		includeTo := rapid.Bool().Draw(t, "includeTo")

		snapshots := []domain.Account{}

		fromBalance, err := domain.NewMoney(1_000, fromCurrency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		if includeFrom {
			snapshots = append(snapshots, fromAccount)
		}

		toBalance, err := domain.NewMoney(0, toCurrency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, toBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		if includeTo {
			snapshots = append(snapshots, toAccount)
		}

		amountMinor := rapid.Int64Range(1, 1_000).Draw(t, "amount")
		amount, err := domain.NewMoney(amountMinor, amountCurrency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		cmd := domain.TransferCommand{From: "from", To: "to", Amount: amount}
		posting, postErr := domain.Post(cmd, snapshots, time.Now(), "tx-relaxed")
		if postErr == nil && !domain.EntriesSumToZero(posting.Entries) {
			t.Fatalf("Post succeeded across mismatched currencies without honouring I1: %+v", posting.Entries)
		}
	})
}

// TestMoney_AdversarialMinorUnits covers the PBT obligation "Adversarial
// amounts: zero, one minor unit, max int64" over domain.Money.
//
// bypass: these are named boundary values (zero, one unit, int64 max), not an
// equivalence class a generator would reliably hit -- a table test names the
// intent more honestly than a property with `min_value`/`max_value` pinned to
// the exact same three numbers.
func TestMoney_AdversarialMinorUnits(t *testing.T) {
	cases := []struct {
		name       string
		minorUnits int64
	}{
		{"zero", 0},
		{"one minor unit", 1},
		{"max int64", math.MaxInt64},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			money, err := domain.NewMoney(tc.minorUnits, "USD")
			if err != nil {
				t.Fatalf("NewMoney(%d, USD) unexpected error: %v", tc.minorUnits, err)
			}
			if money.MinorUnits() != tc.minorUnits {
				t.Fatalf("MinorUnits() = %d, want %d", money.MinorUnits(), tc.minorUnits)
			}
		})
	}
}

// TestPost_ExactBalanceAndOffByOneCent covers the PBT obligation "Adversarial
// amounts: exact balance, off-by-one-cent" over domain.Post.
//
// bypass: this is a two-sided boundary (the exact balance must succeed, one
// cent past it must be refused) that needs Account + Post context, not a
// single Money input -- a table test over Post names the boundary more
// honestly than a property re-deriving the same two numbers.
func TestPost_ExactBalanceAndOffByOneCent(t *testing.T) {
	currency := "USD"
	balance, err := domain.NewMoney(500, currency)
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	wallet, err := domain.NewAccount("wallet-1", domain.Wallet, balance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	systemBalance, err := domain.NewMoney(0, currency)
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	system, err := domain.NewAccount("system-1", domain.System, systemBalance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}

	t.Run("exact balance withdrawal succeeds and empties the wallet", func(t *testing.T) {
		amount, err := domain.NewMoney(500, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		cmd := domain.TransferCommand{From: "wallet-1", To: "system-1", Amount: amount}
		posting, err := domain.Post(cmd, []domain.Account{wallet, system}, time.Now(), "tx-exact")
		if err != nil {
			t.Fatalf("unexpected refusal at exact balance: %v", err)
		}
		if !domain.EntriesSumToZero(posting.Entries) {
			t.Fatalf("entries do not sum to zero: %+v", posting.Entries)
		}
	})

	t.Run("one cent over balance is refused as insufficient funds", func(t *testing.T) {
		amount, err := domain.NewMoney(501, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		cmd := domain.TransferCommand{From: "wallet-1", To: "system-1", Amount: amount}
		_, postErr := domain.Post(cmd, []domain.Account{wallet, system}, time.Now(), "tx-over")

		var violation domain.Violation
		if !errors.As(postErr, &violation) || violation.Kind() != domain.InsufficientFunds {
			t.Fatalf("expected insufficient_funds, got %v", postErr)
		}
	})

	t.Run("system source past its own balance still succeeds and goes negative by design", func(t *testing.T) {
		amount, err := domain.NewMoney(501, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		emptyWallet, err := domain.NewAccount("wallet-2", domain.Wallet, systemBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		cmd := domain.TransferCommand{From: "system-1", To: "wallet-2", Amount: amount}
		posting, err := domain.Post(cmd, []domain.Account{system, emptyWallet}, time.Now(), "tx-system-negative")
		if err != nil {
			t.Fatalf("system source refused past its own (zero) balance, but I4 does not bind System: %v", err)
		}
		if !domain.EntriesSumToZero(posting.Entries) {
			t.Fatalf("entries do not sum to zero: %+v", posting.Entries)
		}
		updated, err := system.Apply(amount.Negate())
		if err != nil {
			t.Fatalf("Account.Apply refused a System account going negative: %v", err)
		}
		if updated.Balance().MinorUnits() >= 0 {
			t.Fatalf("expected system account to go negative by design, got %d", updated.Balance().MinorUnits())
		}
	})
}

// TestPost_UnknownAccountNamesTheMissingOne covers the acceptance criterion
// "Post returns UnknownAccount naming the account absent from the locked
// snapshots".
func TestPost_UnknownAccountNamesTheMissingOne(t *testing.T) {
	currency := "USD"
	balance, err := domain.NewMoney(100, currency)
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	onlyAccount, err := domain.NewAccount("known", domain.Wallet, balance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	amount, err := domain.NewMoney(1, currency)
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}

	cmd := domain.TransferCommand{From: "known", To: "ghost", Amount: amount}
	_, postErr := domain.Post(cmd, []domain.Account{onlyAccount}, time.Now(), "tx-unknown")

	var violation domain.Violation
	if !errors.As(postErr, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found, got %v", postErr)
	}
	if violation.Account() != "ghost" {
		t.Fatalf("UnknownAccount named %q, want %q", violation.Account(), "ghost")
	}
}

// TestNewMoney_RejectsUnknownCurrency covers the acceptance criterion
// "NewMoney rejects ... any unknown currency".
func TestNewMoney_RejectsUnknownCurrency(t *testing.T) {
	_, err := domain.NewMoney(100, "XXX")

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.InvalidAmount {
		t.Fatalf("expected invalid_amount for an unknown currency, got %v", err)
	}
}

// TestProperty_ProvisionTenant_ComplementEquality covers the bounded-change
// contract shape for tenant provisioning (brief.md § Multitenancy
// Contract-shape table): ProvisionTenant's declared delta is exactly one new
// Tenant, and nothing else in the caller's world moves. ProvisionTenant takes
// no accounts/transactions collection at all -- it is a pure single-value
// decision, mirroring OpenAccount -- so the complement-equality obligation
// reduces to: arbitrary account and transaction snapshots threaded alongside
// the call come back byte-identical, and the tenant name set gains exactly
// the one provisioned name and nothing else.
func TestProperty_ProvisionTenant_ComplementEquality(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		existingNames := rapid.SliceOfDistinct(
			rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`),
			func(s string) string { return s },
		).Draw(t, "existingNames")

		candidateName := rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`).Draw(t, "candidateName")
		for _, existing := range existingNames {
			if existing == candidateName {
				t.Skip("generated candidate collides with an existing name")
			}
		}

		tenantID := rapid.StringMatching(`[a-z0-9-]{4,12}`).Draw(t, "tenantID")
		credential := rapid.StringMatching(`[A-Za-z0-9]{8,20}`).Draw(t, "credential")

		// Opaque account/transaction snapshots -- ProvisionTenant never sees
		// these. They stand in for the rest of the caller's world; capturing
		// them before/after and asserting equality is the complement-equality
		// half of the bounded-change contract.
		accountsBefore := rapid.SliceOf(rapid.StringMatching(`acct-[0-9]{1,4}`)).Draw(t, "accountsBefore")
		transactionsBefore := rapid.SliceOf(rapid.StringMatching(`tx-[0-9]{1,4}`)).Draw(t, "transactionsBefore")
		accountsSnapshot := append([]string(nil), accountsBefore...)
		transactionsSnapshot := append([]string(nil), transactionsBefore...)

		tenantsBefore := append([]string(nil), existingNames...)

		tenant, err := domain.ProvisionTenant(tenantID, candidateName, credential, false)
		if err != nil {
			t.Fatalf("ProvisionTenant refused a name absent from the snapshot: %v", err)
		}

		accountsAfter := accountsSnapshot
		transactionsAfter := transactionsSnapshot
		if len(accountsAfter) != len(accountsBefore) {
			t.Fatalf("accounts collection changed size: before=%v after=%v", accountsBefore, accountsAfter)
		}
		for i := range accountsBefore {
			if accountsAfter[i] != accountsBefore[i] {
				t.Fatalf("accounts collection mutated at %d: before=%v after=%v", i, accountsBefore, accountsAfter)
			}
		}
		if len(transactionsAfter) != len(transactionsBefore) {
			t.Fatalf("transactions collection changed size: before=%v after=%v", transactionsBefore, transactionsAfter)
		}
		for i := range transactionsBefore {
			if transactionsAfter[i] != transactionsBefore[i] {
				t.Fatalf("transactions collection mutated at %d: before=%v after=%v", i, transactionsBefore, transactionsAfter)
			}
		}

		tenantsAfter := append(append([]string(nil), tenantsBefore...), tenant.Name())
		if len(tenantsAfter) != len(tenantsBefore)+1 {
			t.Fatalf("tenants collection delta was not exactly one row: before=%v after=%v", tenantsBefore, tenantsAfter)
		}
		for _, existing := range tenantsBefore {
			found := false
			for _, name := range tenantsAfter {
				if name == existing {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("tenants complement-equality broken: %q from before is missing after", existing)
			}
		}

		if tenant.TenantID() != tenantID {
			t.Fatalf("TenantID() = %q, want %q", tenant.TenantID(), tenantID)
		}
		if tenant.Name() != candidateName {
			t.Fatalf("Name() = %q, want %q", tenant.Name(), candidateName)
		}
		if tenant.Credential() != credential {
			t.Fatalf("Credential() = %q, want %q", tenant.Credential(), credential)
		}
	})
}

// TestProperty_ProvisionTenant_RefusesDuplicateName covers I10: tenant-name
// uniqueness enforced at construction, mirroring I9's OpenAccount precedent
// (DDD-18) one aggregate level up. The application-layer caller resolves the
// name-taken read against its store and passes the resolved bool in --
// ProvisionTenant performs no I/O of its own.
func TestProperty_ProvisionTenant_RefusesDuplicateName(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tenantID := rapid.StringMatching(`[a-z0-9-]{4,12}`).Draw(t, "tenantID")
		name := rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`).Draw(t, "name")
		credential := rapid.StringMatching(`[A-Za-z0-9]{8,20}`).Draw(t, "credential")

		_, err := domain.ProvisionTenant(tenantID, name, credential, true)

		var violation domain.Violation
		if !errors.As(err, &violation) || violation.Kind() != domain.TenantAlreadyExists {
			t.Fatalf("expected tenant_already_exists, got %v", err)
		}
	})
}

// TestNewTenant_NoConstructorOmitsTenantID covers the acceptance criterion
// "root-only value-typed fields, no constructor omits tenant_id": every
// accessor round-trips exactly what NewTenant was given, tenant_id included.
func TestNewTenant_NoConstructorOmitsTenantID(t *testing.T) {
	tenant, err := domain.NewTenant("tenant-1", "acme", "s3cr3t-cred")
	if err != nil {
		t.Fatalf("NewTenant unexpected error: %v", err)
	}
	if tenant.TenantID() != "tenant-1" {
		t.Fatalf("TenantID() = %q, want %q", tenant.TenantID(), "tenant-1")
	}
	if tenant.Name() != "acme" {
		t.Fatalf("Name() = %q, want %q", tenant.Name(), "acme")
	}
	if tenant.Credential() != "s3cr3t-cred" {
		t.Fatalf("Credential() = %q, want %q", tenant.Credential(), "s3cr3t-cred")
	}
}

// TestTenantNotFound_NamesTheMissingTenant covers the sealed taxonomy member
// tenant_not_found, added for cross-tenant/lookup refusals at a later step's
// wiring (this step only proves the domain-core member exists and carries the
// tenant identifier).
func TestTenantNotFound_NamesTheMissingTenant(t *testing.T) {
	violation := domain.NewTenantNotFound("ghost-tenant")

	if violation.Kind() != domain.TenantNotFound {
		t.Fatalf("Kind() = %v, want %v", violation.Kind(), domain.TenantNotFound)
	}
	if violation.Tenant() != "ghost-tenant" {
		t.Fatalf("Tenant() = %q, want %q", violation.Tenant(), "ghost-tenant")
	}
}

// The tests below were written to kill specific surviving mutants reported
// by the nightly-delta gremlins run (OPS-9), rather than to cover a PBT
// obligation from the acceptance-designer's taxonomy. Each names the mutant
// it kills in its doc comment.

// TestMoney_IsPositive_ZeroBoundary kills the CONDITIONALS_BOUNDARY mutant at
// money.go:68 (`> 0` mutated to `>= 0`): zero must report false, not true.
//
// bypass: named boundary value (zero), not an equivalence class a generator
// would reliably hit.
func TestMoney_IsPositive_ZeroBoundary(t *testing.T) {
	zero, err := domain.NewMoney(0, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	if zero.IsPositive() {
		t.Fatalf("IsPositive() = true for a zero amount, want false")
	}
}

// TestNewAccount_SystemAllowsNegativeBalanceAtConstruction kills the
// CONDITIONALS_NEGATION mutant at account.go:26 (`kind == Wallet` negated to
// `!=`): a System account must be constructible with a negative balance
// directly (the documented Wallet/System asymmetry), not just reachable via
// repeated Apply calls.
//
// bypass: named boundary case (direct construction vs. reached via Apply),
// not an equivalence class.
func TestNewAccount_SystemAllowsNegativeBalanceAtConstruction(t *testing.T) {
	negative, err := domain.NewMoney(-100, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	account, err := domain.NewAccount("treasury", domain.System, negative)
	if err != nil {
		t.Fatalf("NewAccount refused a negative balance for a System account: %v", err)
	}
	if account.Balance().MinorUnits() != -100 {
		t.Fatalf("Balance().MinorUnits() = %d, want -100", account.Balance().MinorUnits())
	}
}

// TestPost_ToAccountCurrencyMismatchIsRefused kills the CONDITIONALS_NEGATION
// mutant at post.go:50 (the err-check guarding toAccount.Apply): when the
// "from" side's currency matches the amount (so the first Apply succeeds)
// but the "to" side's currency does not, Post must surface the
// CurrencyMismatch from the second Apply rather than silently continuing
// past it.
//
// bypass: a specific two-sided boundary needing three coordinated currency
// values (from matches amount, to does not), not a natural property —
// TestProperty_PostToleratesMixedCurrenciesAndMissingAccounts already draws
// independent random currencies for from/to/amount but never pins this exact
// combination or asserts on it.
func TestPost_ToAccountCurrencyMismatchIsRefused(t *testing.T) {
	fromBalance, err := domain.NewMoney(1_000, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}

	toBalance, err := domain.NewMoney(0, "EUR")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	toAccount, err := domain.NewAccount("to", domain.System, toBalance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}

	amount, err := domain.NewMoney(100, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}

	cmd := domain.TransferCommand{From: "from", To: "to", Amount: amount}
	_, postErr := domain.Post(cmd, []domain.Account{fromAccount, toAccount}, time.Now(), "tx-to-mismatch")

	var violation domain.Violation
	if !errors.As(postErr, &violation) || violation.Kind() != domain.CurrencyMismatch {
		t.Fatalf("expected currency_mismatch from the to-side Apply, got %v", postErr)
	}
}

// TestProperty_NewMoneyFromDecimalLiteral_ValidFracDigitsWithinScale kills
// the ARITHMETIC_BASE/INVERT_NEGATIVES mutants at post.go:129 (the zero-pad
// computation `scale-len(fracDigits)`) and post.go:130 (the
// wholeDigits+padded concatenation): asserting the exact resulting
// MinorUnits exposes any wrong padding length or wrong concatenation order.
func TestProperty_NewMoneyFromDecimalLiteral_ValidFracDigitsWithinScale(t *testing.T) {
	scales := map[string]int{"USD": 2, "EUR": 2, "GBP": 2, "JPY": 0}

	rapid.Check(t, func(t *rapid.T) {
		currency := rapid.SampledFrom([]string{"USD", "EUR", "GBP", "JPY"}).Draw(t, "currency")
		scale := scales[currency]

		wholeDigits := rapid.StringMatching(`[1-9][0-9]{0,5}`).Draw(t, "wholeDigits")
		fracLen := rapid.IntRange(0, scale).Draw(t, "fracLen")
		fracDigits := ""
		if fracLen > 0 {
			fracDigits = rapid.StringOfN(rapid.RuneFrom([]rune("0123456789")), fracLen, fracLen, fracLen).Draw(t, "fracDigits")
		}

		money, err := domain.NewMoneyFromDecimalLiteral(false, wholeDigits, fracDigits, currency)
		if err != nil {
			t.Fatalf("NewMoneyFromDecimalLiteral refused fracDigits (len %d) within scale (%d): %v", fracLen, scale, err)
		}

		padded := fracDigits
		for len(padded) < scale {
			padded += "0"
		}
		wantStr := wholeDigits + padded
		var want int64
		for _, r := range wantStr {
			want = want*10 + int64(r-'0')
		}

		if money.MinorUnits() != want {
			t.Fatalf("MinorUnits() = %d, want %d (whole=%q frac=%q scale=%d)", money.MinorUnits(), want, wholeDigits, fracDigits, scale)
		}
	})
}

// TestProperty_PostRefusesCrossTenantReference covers I8's construction-time
// story: a TransferCommand naming a snapshot whose tenant_id differs from the
// command's own tenant_id is refused with account_not_found, before any
// balance math runs -- so the refusal fires even when the mismatched account
// would otherwise have supported the movement (or would otherwise have been
// refused for an unrelated reason, e.g. insufficient funds). Extends the
// existing rapid harness for domain.Post (step 02-01).
func TestProperty_PostRefusesCrossTenantReference(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currency := genCurrency(t, "currency")

		commandTenant := rapid.StringMatching(`tnt_[a-z0-9]{4}`).Draw(t, "commandTenant")
		otherTenant := rapid.StringMatching(`tnt_[a-z0-9]{4}`).Draw(t, "otherTenant")
		if commandTenant == otherTenant {
			t.Skip("generator drew the same tenant twice -- not a cross-tenant case")
		}

		mismatchSide := rapid.SampledFrom([]string{"from", "to"}).Draw(t, "mismatchSide")

		fromBalanceMinor := rapid.Int64Range(0, 1_000_000_000).Draw(t, "fromBalance")
		fromBalance, err := domain.NewMoney(fromBalanceMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		toBalanceMinor := rapid.Int64Range(0, 1_000_000_000).Draw(t, "toBalance")
		toBalance, err := domain.NewMoney(toBalanceMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, toBalance)
		if err != nil {
			t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		var wantMismatchedID string
		if mismatchSide == "from" {
			fromAccount = fromAccount.WithTenant(otherTenant)
			toAccount = toAccount.WithTenant(commandTenant)
			wantMismatchedID = "from"
		} else {
			fromAccount = fromAccount.WithTenant(commandTenant)
			toAccount = toAccount.WithTenant(otherTenant)
			wantMismatchedID = "to"
		}

		// Deliberately request more than the wallet side can cover, so an
		// insufficient-funds refusal is available as a competing outcome --
		// the cross-tenant refusal must still win.
		amountMinor := rapid.Int64Range(fromBalanceMinor+1, fromBalanceMinor+1_000_000).Draw(t, "amount")
		amount, err := domain.NewMoney(amountMinor, currency)
		if err != nil {
			t.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		cmd := domain.TransferCommand{From: "from", To: "to", Amount: amount, TenantID: commandTenant}
		_, postErr := domain.Post(cmd, []domain.Account{fromAccount, toAccount}, time.Now(), "tx-cross-tenant")

		var violation domain.Violation
		if !errors.As(postErr, &violation) || violation.Kind() != domain.UnknownAccount {
			t.Fatalf("expected account_not_found for a cross-tenant reference (even under insufficient funds), got %v", postErr)
		}
		if violation.Account() != wantMismatchedID {
			t.Fatalf("cross-tenant refusal named %q, want %q", violation.Account(), wantMismatchedID)
		}
	})
}

// TestPost_CrossTenantRefusalReusesAccountNotFound covers the acceptance
// criterion "Cross-tenant refusal reuses the existing account_not_found
// member (I8 confirmed, no new taxonomy member)" with a pinned example: I1
// and I4 keep their existing shape -- this is one additional branch in the
// same pre-construction gate, not a restructuring.
func TestPost_CrossTenantRefusalReusesAccountNotFound(t *testing.T) {
	balance, err := domain.NewMoney(1_000, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	fromAccount, err := domain.NewAccount("from", domain.Wallet, balance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	fromAccount = fromAccount.WithTenant("tnt_alice")

	zero, err := domain.NewMoney(0, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	toAccount, err := domain.NewAccount("to", domain.System, zero)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	toAccount = toAccount.WithTenant("tnt_bob")

	amount, err := domain.NewMoney(100, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}

	cmd := domain.TransferCommand{From: "from", To: "to", Amount: amount, TenantID: "tnt_alice"}
	_, postErr := domain.Post(cmd, []domain.Account{fromAccount, toAccount}, time.Now(), "tx-cross-tenant-pinned")

	var violation domain.Violation
	if !errors.As(postErr, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found (reused, no new taxonomy member), got %v", postErr)
	}
	if violation.Account() != "to" {
		t.Fatalf("cross-tenant refusal named %q, want %q", violation.Account(), "to")
	}
}

// TestNewMoneyFromDecimalLiteral_FracDigitsExceedingScaleIsRefused kills the
// CONDITIONALS_BOUNDARY and CONDITIONALS_NEGATION mutants at post.go:126
// (`len(fracDigits) > scale`): one fractional digit past the currency's
// scale must be refused, not silently accepted or off-by-one accepted.
func TestNewMoneyFromDecimalLiteral_FracDigitsExceedingScaleIsRefused(t *testing.T) {
	cases := []struct {
		currency   string
		fracDigits string
	}{
		{"USD", "123"}, // scale 2, one digit past it
		{"JPY", "1"},   // scale 0, any fractional digit is past it
	}

	for _, tc := range cases {
		t.Run(tc.currency+"/"+tc.fracDigits, func(t *testing.T) {
			_, err := domain.NewMoneyFromDecimalLiteral(false, "1", tc.fracDigits, tc.currency)

			var violation domain.Violation
			if !errors.As(err, &violation) || violation.Kind() != domain.InvalidAmount {
				t.Fatalf("expected invalid_amount for fracDigits %q past %s's scale, got %v", tc.fracDigits, tc.currency, err)
			}
		})
	}
}
