package multitenancy

import (
	"context"

	"github.com/cucumber/godog"
)

// Step definitions. Per Mandate-12 every body coerces its captured text into
// a domain type in argument position and delegates to a World method — no
// branching, no loops, no business logic in a step body. Mirrors
// tests/acceptance/ledgercore/steps_ledger_test.go's own convention exactly.

func RegisterSteps(ctx *godog.ScenarioContext, w *World) {

	// --- Given: identity and starting state --------------------------------

	ctx.Given(`^the operator holds the platform-admin credential$`, func() error {
		w.ActAs(PlatformAdmin())
		return nil
	})

	ctx.Given(`^a fresh store with no tenants provisioned$`, func(c context.Context) error {
		return w.EnsureStarted(c)
	})

	ctx.Given(`^tenant "([^"]*)" has been provisioned$`, func(c context.Context, name string) error {
		return w.GivenTenantProvisioned(c, TenantName(name))
	})

	ctx.Given(`^no tenant named "([^"]*)" has been provisioned$`, func(c context.Context, _ string) error {
		return w.EnsureStarted(c)
	})

	ctx.Given(`^the caller holds tenant "([^"]*)"'s own credential, not the platform-admin credential$`,
		func(name string) error {
			w.ActAs(AsTenant(TenantName(name)))
			return nil
		})

	ctx.Given(`^the caller presents an unissued credential$`, func() error {
		w.ActAs(UnissuedCredential())
		return nil
	})

	ctx.Given(`^no caller credential is presented$`, func() error {
		w.ActAs(NoCredential())
		return nil
	})

	ctx.Given(`^the stored balance of "([^"]*)" is altered by (\S+) out of band$`,
		func(c context.Context, name, amount string) error {
			return w.CorruptStoredBalance(c, AccountName(name), ParseMoney(amount))
		})

	// --- When: provisioning (slice 01) --------------------------------------

	ctx.When(`^the operator provisions (?:a|another) tenant named "([^"]*)"$`, func(c context.Context, name string) error {
		w.ActAs(PlatformAdmin())
		return w.ProvisionTenant(c, TenantName(name))
	})

	ctx.When(`^that caller attempts to provision a tenant named "([^"]*)"$`, func(c context.Context, name string) error {
		return w.ProvisionTenant(c, TenantName(name))
	})

	// --- When: operate within a tenant (slice 02) ---------------------------

	ctx.When(`^tenant "([^"]*)" opens an? (wallet|system) account named "([^"]*)"$`,
		func(c context.Context, tenant, kind, name string) error {
			return w.OpenAccount(c, TenantName(tenant), AccountName(name), ParseAccountKind(kind))
		})

	ctx.Given(`^tenant "([^"]*)" opens an? (wallet|system) account named "([^"]*)"$`,
		func(c context.Context, tenant, kind, name string) error {
			return w.OpenAccount(c, TenantName(tenant), AccountName(name), ParseAccountKind(kind))
		})

	ctx.When(`^that caller attempts to open an account$`, func(c context.Context) error {
		return w.OpenAccountAs(c, w.actingAs, AccountName("probe-account"), Wallet)
	})

	ctx.When(`^tenant "([^"]*)" funds "([^"]*)" with (\S+) from "([^"]*)"$`,
		func(c context.Context, tenant, to, amount, from string) error {
			return w.FundAccount(c, TenantName(tenant), AccountName(to), ParseMoney(amount), AccountName(from))
		})

	ctx.Given(`^tenant "([^"]*)" funds "([^"]*)" with (\S+) from "([^"]*)"$`,
		func(c context.Context, tenant, to, amount, from string) error {
			return w.FundAccount(c, TenantName(tenant), AccountName(to), ParseMoney(amount), AccountName(from))
		})

	ctx.When(`^tenant "([^"]*)" requests tenant "[^"]*"'s account "([^"]*)"$`,
		func(c context.Context, tenant, account string) error {
			return w.RequestAccount(c, AsTenant(TenantName(tenant)), AccountName(account))
		})

	ctx.When(`^tenant "([^"]*)" requests tenant "[^"]*"'s entries for "([^"]*)"$`,
		func(c context.Context, tenant, account string) error {
			return w.RequestEntries(c, AsTenant(TenantName(tenant)), AccountName(account))
		})

	ctx.When(`^the operator requests "([^"]*)"'s entries with the platform-admin credential, unscoped$`,
		func(c context.Context, account string) error {
			return w.RequestEntries(c, PlatformAdmin(), AccountName(account))
		})

	ctx.When(`^tenant "([^"]*)" moves (\S+) from "([^"]*)" to "([^"]*)"$`,
		func(c context.Context, tenant, amount, from, to string) error {
			return w.Transfer(c, TenantName(tenant), AccountName(from), AccountName(to), ParseMoney(amount))
		})

	ctx.When(`^tenant "([^"]*)" moves (\S+) from "([^"]*)" to tenant "([^"]*)"'s account "([^"]*)"$`,
		func(c context.Context, tenant, amount, from, toTenant, to string) error {
			return w.TransferAcrossTenants(c, TenantName(tenant), AccountName(from), TenantName(toTenant), AccountName(to))
		})

	// --- When: verify one tenant's books (slice 03) -------------------------

	ctx.When(`^the operator checks the trial balance for tenant "([^"]*)"$`, func(c context.Context, tenant string) error {
		return w.CheckTrialBalance(c, TenantName(tenant))
	})

	ctx.When(`^the operator checks the trial balance unscoped$`, func(c context.Context) error {
		return w.CheckTrialBalance(c, "")
	})

	ctx.When(`^the operator checks the console verdict$`, func(c context.Context) error {
		return w.CheckConsoleVerdict(c)
	})

	// --- Then: provisioning --------------------------------------------------

	ctx.Then(`^the response names tenant "([^"]*)" with a distinct tenant id and tenant credential$`,
		func(name string) error {
			return w.AssertTenantProvisioned(TenantName(name))
		})

	ctx.Then(`^the tenant credential is distinct from the platform-admin credential$`, func() error {
		return w.AssertTenantKeyDistinctFromAdmin()
	})

	ctx.Then(`^tenant "([^"]*)" remains provisioned with its original credential$`, func(name string) error {
		return w.AssertTenantStillProvisioned(TenantName(name))
	})

	ctx.Then(`^the response is refused as a tenant that already exists$`, func() error {
		return w.AssertTenantRefused(TenantAlreadyExists)
	})

	ctx.Then(`^no second tenant is created$`, func() error {
		return w.AssertNoNewTenant()
	})

	ctx.Then(`^no tenant is created$`, func() error {
		return w.AssertNoNewTenant()
	})

	// "the response is refused as unidentified" fires after either a
	// provisioning attempt or an account attempt (milestone-01 / milestone-02
	// respectively) — LastRefusal is the single SSOT both call paths update.
	ctx.Then(`^the response is refused as unidentified$`, func() error {
		return w.AssertRefused(Unidentified)
	})

	// --- Then: operate within a tenant ---------------------------------------

	ctx.Then(`^both accounts are opened successfully$`, func() error {
		return w.AssertBothAccountsOpened()
	})

	ctx.Then(`^tenant "([^"]*)"'s account "([^"]*)" balance reads (\S+)$`, func(c context.Context, tenant, account, amount string) error {
		return w.AssertAccountBalance(c, TenantName(tenant), AccountName(account), ParseMoney(amount))
	})

	ctx.Then(`^the response is refused as an unknown account$`, func() error {
		return w.AssertRefused(UnknownAccount)
	})

	ctx.Then(`^the transfer is refused$`, func() error {
		return w.AssertRefusedGeneric()
	})

	ctx.Then(`^the response succeeds, unchanged in shape from today's single-tenant entries contract$`, func() error {
		return w.AssertEntriesSucceeded()
	})

	// --- Then: verify one tenant's books --------------------------------------

	ctx.Then(`^the verdict states "([^"]*)"$`, func(text string) error {
		return w.AssertVerdict(ParseVerdict(text))
	})

	ctx.Then(`^no mention of tenant "([^"]*)" appears in the response$`, func(name string) error {
		return w.AssertNoMentionOf(TenantName(name))
	})

	ctx.Then(`^the response names "([^"]*)" in the drift listing$`, func(account string) error {
		return w.AssertDriftNames(AccountName(account))
	})

	ctx.Then(`^no account belonging to tenant "([^"]*)" appears in the response$`, func(name string) error {
		return w.AssertNoAccountOf(TenantName(name))
	})

	ctx.Then(`^the response is refused as an unknown tenant$`, func() error {
		return w.AssertRefused(TenantNotFound)
	})

	ctx.Then(`^the response succeeds with a stated verdict, unchanged in shape from today's single-tenant contract$`, func() error {
		return w.AssertBooksSucceeded()
	})
}
