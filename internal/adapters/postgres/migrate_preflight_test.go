package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
		{"no admin option while role exists", func(f *Facts) {
			f.RoleExists, f.HasAdminOptionOnRole, f.RoleOwnerMarker = true, false, OwnerMarker(f.TargetDatabase)
		}, "ADMIN OPTION"},
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
func TestPlanMigration_RefusesAnUnfinishedMigrationFirstNamingItsVersion(t *testing.T) {
	for _, version := range []uint{1, 3, 6} {
		facts := fullyPrivilegedFacts()
		facts.HasLedgerSchema, facts.DirtyVersion = true, &version
		facts.OwnsDatabaseOrGrant = false // the dirty check runs before every privilege check
		plan := PlanMigration(facts, MigrateConfig{})
		if !plan.Refused() || !plan.NoChanges || len(plan.Steps) != 0 {
			t.Fatalf("version %d: plan = %+v, want a refusal with no steps", version, plan)
		}
		want := fmt.Sprintf("migration %d is unfinished (dirty)", version)
		if !strings.Contains(plan.Refusal, want) || !strings.Contains(plan.Refusal, "no changes applied") {
			t.Errorf("refusal %q does not name %q and say no changes applied", plan.Refusal, want)
		}
	}
}

func TestPlanMigration_ClusterRule(t *testing.T) {
	cases := []struct {
		name        string
		marker      string
		ownerExists bool
		hasSchema   bool
		refused     bool
		names       []string
	}{
		{"role marked for another live database", OwnerMarker("kestrel_ledger"), true, false, true,
			[]string{"ledgerops_app", "kestrel_ledger in this cluster", "separate cluster", "DROP ROLE ledgerops_app", "no changes applied"}},
		{"role marked for a dropped database", OwnerMarker("kestrel_ledger"), false, false, true,
			[]string{"kestrel_ledger, which no longer exists", "DROP ROLE ledgerops_app"}},
		{"role from the demo stack without a marker", "", false, false, true,
			[]string{"another ledgerops database in this cluster", "DROP ROLE ledgerops_app"}},
		{"abandoned claim of the target itself", OwnerMarker("kestrel_ledger_2"), false, false, false, nil},
		{"target already holds the ledger schema", OwnerMarker("kestrel_ledger"), true, true, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := fullyPrivilegedFacts()
			facts.TargetDatabase = "kestrel_ledger_2"
			facts.RoleExists, facts.RoleOwnerMarker, facts.OwnerDatabaseExists, facts.HasLedgerSchema = true, tc.marker, tc.ownerExists, tc.hasSchema
			plan := PlanMigration(facts, MigrateConfig{})
			if plan.Refused() != tc.refused {
				t.Fatalf("refused = %v, want %v: %+v", plan.Refused(), tc.refused, plan)
			}
			for _, want := range tc.names {
				if !strings.Contains(plan.Refusal, want) {
					t.Errorf("refusal %q does not name %q", plan.Refusal, want)
				}
			}
		})
	}
	if PlanMigration(fullyPrivilegedFacts(), MigrateConfig{}).Refused() {
		t.Error("the first ledgerops database in a cluster was refused")
	}
}

func TestSingleStatementDSN_NeverCarriesMultiStatement(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@h:5432/db?sslmode=disable":                        "postgres://u:p@h:5432/db?sslmode=disable",
		"postgres://u:p@h:5432/db?x-multi-statement=true&sslmode=disable": "postgres://u:p@h:5432/db?sslmode=disable",
		"postgres://u:p@h:5432/db?x-multi-statement=true":                 "postgres://u:p@h:5432/db",
	}
	for in, want := range cases {
		if got := singleStatementDSN(in); got != want {
			t.Errorf("singleStatementDSN(%q) = %q, want %q", in, got, want)
		}
	}
}

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

func TestClaimAppRole_ConcurrentClaimsFromTwoDatabasesLeaveExactlyOneOwner(t *testing.T) {
	ctx, dsn := startPlainPostgres(t)
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, `CREATE DATABASE other_db`); err != nil {
		t.Fatalf("create second database: %v", err)
	}
	dsns := []string{dsn, strings.Replace(dsn, "/probe_db", "/other_db", 1)}

	for round := 0; round < 20; round++ {
		if _, err := admin.Exec(ctx, `DROP ROLE IF EXISTS ledgerops_app`); err != nil {
			t.Fatalf("reset: %v", err)
		}
		start := make(chan struct{})
		errs := make([]error, len(dsns))
		var wg sync.WaitGroup
		for i, d := range dsns {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = claimAppRole(ctx, d)
			}()
		}
		close(start)
		wg.Wait()

		wins, refusals := 0, 0
		for _, err := range errs {
			var refusal *RefusalError
			switch {
			case err == nil:
				wins++
			case errors.As(err, &refusal) && strings.Contains(refusal.Plan.Refusal, "DROP ROLE ledgerops_app"):
				refusals++
			default:
				t.Fatalf("round %d: unexpected claim error: %v", round, err)
			}
		}
		if wins != 1 || refusals != 1 {
			t.Fatalf("round %d: %d claims won and %d refused, want exactly one of each: %v", round, wins, refusals, errs)
		}
		// A repeat claim from the winner's own database still succeeds.
		var marker string
		if err := admin.QueryRow(ctx, `SELECT shobj_description(oid, 'pg_authid') FROM pg_roles WHERE rolname = 'ledgerops_app'`).Scan(&marker); err != nil {
			t.Fatalf("round %d: reading marker: %v", round, err)
		}
		winner := dsns[0]
		if markedOwner(marker) == "other_db" {
			winner = dsns[1]
		}
		if err := claimAppRole(ctx, winner); err != nil {
			t.Fatalf("round %d: re-claim from the owning database refused: %v", round, err)
		}
	}
}
