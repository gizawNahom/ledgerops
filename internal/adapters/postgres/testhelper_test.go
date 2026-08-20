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
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("ledgerops"),
		tcpostgres.WithUsername("ledgerops_migrate"),
		tcpostgres.WithPassword("migrate-secret"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second),
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
	return store
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
