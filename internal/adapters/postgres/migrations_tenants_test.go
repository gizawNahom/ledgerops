// Migration-verification integration test for step 01-01 (ADR-013, DDD-24).
// Real PostgreSQL 16 via Testcontainers (OPS-11, Mandate 6) -- a single-
// example wiring test, per the layered test discipline
// (nw-tdd-methodology § Layered test discipline): "Integration | UNCHANGED —
// single-example test verifies WIRING."
//
// This step has no test_file / scenario_name -- it is pure infrastructure,
// not directly targeted by an acceptance scenario -- so this file is the
// step's own RED->GREEN proof: it fails for a real reason (tenants doesn't
// exist / accounts' PK isn't composite) before migration 0003 exists, and
// passes once migration 0003's SQL lands.
package postgres_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// legacyBackfilledCluster brings up PostgreSQL 16, applies migrations 0001
// and 0002 only (the pre-tenants schema), inserts one account row the way
// today's already-shipped AccountRepository.Create does -- no tenant_id
// mentioned, because that column does not exist yet at this point in the
// sequence -- then applies every remaining migration (0003 included) to
// bring the cluster to head. This is what lets the test assert the
// "pre-existing rows backfill to tnt_legacy_seed" criterion against a row
// that genuinely predates the tenants table, rather than one seeded after.
func legacyBackfilledCluster(t *testing.T) (privilegedDSN string) {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("ledgerops"),
		tcpostgres.WithUsername("ledgerops_migrate"),
		tcpostgres.WithPassword("migrate-secret"),
		testcontainers.WithWaitStrategy(
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

	privilegedDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("reading the privileged connection string: %v", err)
	}

	m, err := migrate.New("file://migrations", privilegedDSN)
	if err != nil {
		t.Fatalf("opening the migrator: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Steps(2); err != nil {
		t.Fatalf("applying migrations 0001-0002 (pre-tenants schema): %v", err)
	}

	conn, err := pgx.Connect(ctx, privilegedDSN)
	if err != nil {
		t.Fatalf("connecting to seed a pre-existing account: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO accounts (id, kind, balance_minor, currency) VALUES ($1, $2, $3, $4)`,
		"legacy-wallet-01", "wallet", 1000, "USD"); err != nil {
		conn.Close(ctx)
		t.Fatalf("seeding a pre-existing account before migration 0003: %v", err)
	}
	conn.Close(ctx)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("applying the remaining migrations (0003 included): %v", err)
	}

	return privilegedDSN
}

// constraintDefs returns every constraint definition on the given table,
// ordered by constraint name, as rendered by pg_get_constraintdef -- e.g.
// "FOREIGN KEY (tenant_id, account_id) REFERENCES accounts(tenant_id, id)".
func constraintDefs(t *testing.T, conn *pgx.Conn, table string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid = $1::regclass ORDER BY conname`, table)
	if err != nil {
		t.Fatalf("reading constraints for %q: %v", table, err)
	}
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatalf("scanning a constraint definition for %q: %v", table, err)
		}
		defs = append(defs, def)
	}
	return defs
}

// primaryKeyColumns returns a table's primary-key columns, alphabetically
// sorted, so the assertion does not depend on declaration order.
func primaryKeyColumns(t *testing.T, conn *pgx.Conn, table string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT a.attname FROM pg_index i
		 JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		 WHERE i.indrelid = $1::regclass AND i.indisprimary`, table)
	if err != nil {
		t.Fatalf("reading the primary key columns for %q: %v", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scanning a primary key column for %q: %v", table, err)
		}
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// grantedPrivileges returns the privileges granted to grantee on table,
// alphabetically sorted.
func grantedPrivileges(t *testing.T, conn *pgx.Conn, table, grantee string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT privilege_type FROM information_schema.role_table_grants
		 WHERE table_name = $1 AND grantee = $2`, table, grantee)
	if err != nil {
		t.Fatalf("reading grants for %q on %q: %v", grantee, table, err)
	}
	defer rows.Close()

	var privileges []string
	for rows.Next() {
		var privilege string
		if err := rows.Scan(&privilege); err != nil {
			t.Fatalf("scanning a grant for %q on %q: %v", grantee, table, err)
		}
		privileges = append(privileges, privilege)
	}
	sort.Strings(privileges)
	return privileges
}

func containsSubstring(defs []string, substr string) bool {
	for _, def := range defs {
		if strings.Contains(def, substr) {
			return true
		}
	}
	return false
}

// TestMigration_TenantSchema proves migration 0003's full shape (ADR-013):
// tenants exists with the right constraints and least-privilege grant,
// accounts' primary key is composite, a row that predates the tenants table
// backfills to tnt_legacy_seed, and entries' account/counterparty FKs widen
// to tenant-scoped composites.
func TestMigration_TenantSchema(t *testing.T) {
	privilegedDSN := legacyBackfilledCluster(t)
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, privilegedDSN)
	if err != nil {
		t.Fatalf("connecting to verify the migrated schema: %v", err)
	}
	defer conn.Close(ctx)

	t.Run("tenants table carries the required constraints", func(t *testing.T) {
		defs := constraintDefs(t, conn, "tenants")
		if !containsSubstring(defs, "PRIMARY KEY (tenant_id)") {
			t.Errorf("tenants constraints = %v, want a PRIMARY KEY (tenant_id)", defs)
		}
		if !containsSubstring(defs, "UNIQUE (name)") {
			t.Errorf("tenants constraints = %v, want UNIQUE (name)", defs)
		}
		if !containsSubstring(defs, "UNIQUE (credential_hash)") {
			t.Errorf("tenants constraints = %v, want UNIQUE (credential_hash)", defs)
		}
	})

	t.Run("ledgerops_app holds only SELECT and INSERT on tenants", func(t *testing.T) {
		got := grantedPrivileges(t, conn, "tenants", "ledgerops_app")
		want := []string{"INSERT", "SELECT"}
		if len(got) != len(want) {
			t.Fatalf("privileges on tenants for ledgerops_app = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("privileges on tenants for ledgerops_app = %v, want %v", got, want)
			}
		}
	})

	t.Run("accounts primary key is composite (tenant_id, id)", func(t *testing.T) {
		got := primaryKeyColumns(t, conn, "accounts")
		want := []string{"id", "tenant_id"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("accounts primary key columns = %v, want %v", got, want)
		}
	})

	t.Run("a pre-existing account backfills to tnt_legacy_seed", func(t *testing.T) {
		var tenantID string
		if err := conn.QueryRow(ctx,
			`SELECT tenant_id FROM accounts WHERE id = $1`, "legacy-wallet-01",
		).Scan(&tenantID); err != nil {
			t.Fatalf("reading the pre-existing account's tenant_id: %v", err)
		}
		if tenantID != "tnt_legacy_seed" {
			t.Fatalf("tenant_id = %q, want %q", tenantID, "tnt_legacy_seed")
		}
	})

	t.Run("entries and transactions gain a NOT NULL tenant_id column", func(t *testing.T) {
		for _, table := range []string{"entries", "transactions"} {
			var isNullable string
			if err := conn.QueryRow(ctx,
				`SELECT is_nullable FROM information_schema.columns
				 WHERE table_name = $1 AND column_name = 'tenant_id'`, table,
			).Scan(&isNullable); err != nil {
				t.Fatalf("reading %s.tenant_id's nullability: %v", table, err)
			}
			if isNullable != "NO" {
				t.Fatalf("%s.tenant_id is_nullable = %q, want %q", table, isNullable, "NO")
			}
		}
	})

	t.Run("entries' account and counterparty FKs are tenant-scoped composites", func(t *testing.T) {
		defs := constraintDefs(t, conn, "entries")
		if !containsSubstring(defs, "FOREIGN KEY (tenant_id, account_id) REFERENCES accounts(tenant_id, id)") {
			t.Errorf("entries constraints = %v, want a composite FK on (tenant_id, account_id)", defs)
		}
		if !containsSubstring(defs, "FOREIGN KEY (tenant_id, counterparty_id) REFERENCES accounts(tenant_id, id)") {
			t.Errorf("entries constraints = %v, want a composite FK on (tenant_id, counterparty_id)", defs)
		}
	})
}
