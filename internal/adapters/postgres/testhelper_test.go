// Adapter integration tests for the driven postgres adapter, real PostgreSQL
// 16 via Testcontainers (OPS-11, Mandate 6). These are wiring tests, not
// property tests, per the layered test discipline
// (nw-tdd-methodology § Layered test discipline): "Integration | UNCHANGED —
// single-example test verifies WIRING."
package postgres_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"ledgerops/internal/adapters/postgres"
	"ledgerops/internal/app/ports"
)

// migratedStore brings up PostgreSQL 16, migrates it from zero as the
// privileged role (step 01-02), and opens the store as the application role
// (OPS-10) — the same shape every repository test in this package needs.
func migratedStore(t *testing.T) ports.Store {
	t.Helper()
	store, _ := migratedStoreWithDSN(t)
	return store
}

// migratedStoreWithDSN is migratedStore plus the application-role DSN, for
// the rare test (TestTenantRepository_Create_NeverStoresThePlaintextCredential)
// that must read a column — credential_hash — no driven port exposes, to
// prove the plaintext credential never reaches it.
func migratedStoreWithDSN(t *testing.T) (ports.Store, string) {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("ledgerops"),
		tcpostgres.WithUsername("ledgerops_migrate"),
		tcpostgres.WithPassword("migrate-secret"),
		testcontainers.WithWaitStrategy(
			// Postgres's official image restarts itself once internally after
			// initdb; the port is briefly listening during that first phase
			// too, so a port-only wait can return "ready" in the narrow window
			// right before the restart and hand back a connection the restart
			// then resets. Waiting for the ready-to-accept-connections log
			// line TWICE (once per boot) is what the module's own default
			// wait strategy does — pinning it explicitly here so the
			// StartupTimeout override doesn't silently drop that protection.
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("bringing up PostgreSQL 16: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	privilegedDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("reading the privileged connection string: %v", err)
	}
	if err := postgres.Migrate(ctx, privilegedDSN); err != nil {
		t.Fatalf("migrating from zero: %v", err)
	}

	parsed, err := url.Parse(privilegedDSN)
	if err != nil {
		t.Fatalf("parsing the privileged DSN: %v", err)
	}
	parsed.User = url.UserPassword("ledgerops_app", "app-secret")
	appDSN := parsed.String()

	store, err := postgres.Open(ctx, appDSN)
	if err != nil {
		t.Fatalf("opening the store as the application role: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store, appDSN
}

// beginUOW is a small convenience so every repository test does not repeat
// the same three lines.
func beginUOW(t *testing.T, store ports.Store) ports.UnitOfWork {
	t.Helper()
	uow, err := store.Begin(context.Background())
	if err != nil {
		t.Fatalf("beginning a unit of work: %v", err)
	}
	return uow
}
