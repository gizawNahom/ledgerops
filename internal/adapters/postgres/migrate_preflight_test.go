package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"ledgerops/internal/support"
)

func fullyPrivilegedFacts() Facts {
	return Facts{
		TargetDatabase: "kestrel_ledger", CanCreateRole: true, HasAdminOptionOnRole: true,
		OwnsDatabaseOrGrant: true, CanCreateOnPublic: true, OwnsLedgerTables: true,
	}
}

func TestPlanMigration_RefusesEachMissingPrivilegeNamingItAndSayingNothingChanged(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*Facts)
		privilege string
	}{
		{"no CONNECT right", func(f *Facts) { f.OwnsDatabaseOrGrant = false }, "CONNECT"},
		{"no CREATE on public", func(f *Facts) { f.CanCreateOnPublic = false }, "CREATE on schema public"},
		{"no CREATEROLE while role absent", func(f *Facts) { f.CanCreateRole = false }, "CREATEROLE"},
		{"no admin option while role exists", func(f *Facts) { f.RoleExists, f.HasAdminOptionOnRole = true, false }, "ADMIN OPTION"},
		{"table owned by another role", func(f *Facts) {
			f.HasLedgerSchema, f.OwnsLedgerTables, f.PendingVersions = true, false, []uint{5, 6}
		}, "table owner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := fullyPrivilegedFacts()
			tc.mutate(&facts)
			plan := PlanMigration(facts, MigrateConfig{})
			if !plan.Refused() || !plan.NoChanges || len(plan.Steps) != 0 {
				t.Fatalf("plan = %+v, want a refusal with no steps and NoChanges", plan)
			}
			if !strings.Contains(plan.Privilege, tc.privilege) || !strings.Contains(plan.Refusal, tc.privilege) {
				t.Errorf("refusal %q does not name %q", plan.Refusal, tc.privilege)
			}
			if !strings.Contains(plan.Refusal, "no changes applied") {
				t.Errorf("refusal %q does not say no changes applied", plan.Refusal)
			}
		})
	}
}

func TestPlanMigration_AcceptedPlanOrdersRoleThenSchemaThenPassword(t *testing.T) {
	plan := PlanMigration(fullyPrivilegedFacts(), MigrateConfig{})
	want := []Step{ClaimRole, ApplySchema, SetAppRolePassword}
	if plan.Refused() || len(plan.Steps) != len(want) {
		t.Fatalf("plan = %+v, want steps %v", plan, want)
	}
	for i := range want {
		if plan.Steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", plan.Steps, want)
		}
	}
}

func TestPlanMigration_UnownedTablesAreIgnoredWhenNothingIsPending(t *testing.T) {
	facts := fullyPrivilegedFacts()
	facts.HasLedgerSchema, facts.OwnsLedgerTables = true, false
	if PlanMigration(facts, MigrateConfig{}).Refused() {
		t.Fatal("refused although no migration is pending")
	}
}

// FactReader exposes only SELECT: structural guard on every statement it may run.
func TestFactReader_RunsOnlySelectStatements(t *testing.T) {
	for _, query := range factQueries {
		if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "SELECT") {
			t.Errorf("fact query is not a SELECT: %s", query)
		}
	}
}

func startPlainPostgres(t *testing.T) (context.Context, string) {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("probe_db"), tcpostgres.WithUsername("probe_admin"), tcpostgres.WithPassword("probe-secret"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	if err != nil {
		t.Fatalf("bringing up PostgreSQL 16: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("reading the connection string: %v", err)
	}
	return ctx, dsn
}

// Adapter probes against real PostgreSQL: the reader reports a superuser's
// rights on an empty database; the write-capable prober answers yes and
// leaves no role and no table behind (always rolled back).
func TestPreflightAdapters_ReadFactsAndLeaveNoTraceFromProbes(t *testing.T) {
	ctx, dsn := startPlainPostgres(t)

	facts, err := pgFactReader{ctx: ctx, privilegedDSN: dsn}.ReadFacts("")
	if err != nil {
		t.Fatalf("ReadFacts: %v", err)
	}
	if facts.TargetDatabase != "probe_db" || facts.RoleExists || facts.HasLedgerSchema ||
		!facts.CanCreateRole || !facts.OwnsDatabaseOrGrant || !facts.CanCreateOnPublic || len(facts.PendingVersions) == 0 {
		t.Fatalf("unexpected facts on an empty database: %+v", facts)
	}

	probed, err := pgPreflightProber{ctx: ctx, privilegedDSN: dsn}.Probe(facts.TargetDatabase)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !probed.CanCreateRole || !probed.CanCreateOnPublic {
		t.Fatalf("probe = %+v, want both rights", probed)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var leftovers int
	if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM pg_roles WHERE rolname = 'ledgerops_preflight_probe')
		+ (SELECT count(*) FROM pg_tables WHERE tablename = 'ledgerops_preflight_probe')`).Scan(&leftovers); err != nil {
		t.Fatalf("counting leftovers: %v", err)
	}
	if leftovers != 0 {
		t.Fatalf("probes left %d objects behind", leftovers)
	}
}

func TestPreflight_UnreachableDatabaseErrorRedactsPassword(t *testing.T) {
	ctx := context.Background()
	_, err := preflight(ctx, "postgres://admin:hunter2secret@127.0.0.1:1/db?sslmode=disable&connect_timeout=2", support.NewSecret("pw"))
	if err == nil || strings.Contains(err.Error(), "hunter2secret") {
		t.Fatalf("err = %v, want a redacted connection error", err)
	}
}
