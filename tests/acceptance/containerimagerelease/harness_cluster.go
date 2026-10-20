package containerimagerelease

// Cluster harness: one Testcontainers postgres:16 per @dedicated-cluster
// scenario (policy row "Cluster-global PostgreSQL state"), the catalog
// snapshot used for state-delta assertions (Mandate 8), and the fixtures the
// scenarios need (frozen legacy database, privilege roles, injected faults).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"ledgerops/internal/adapters/postgres/prefeature"
	"ledgerops/tests/common/statedelta"
)

// Cluster is a dedicated Postgres cluster.
type Cluster struct {
	c        testcontainers.Container
	host     string
	port     string
	User     string
	Password string
	Initial  string
}

// StartCluster boots postgres:16 with a superuser and an initial database.
func StartCluster(ctx context.Context, user, password, database string, logAll bool) (*Cluster, error) {
	opts := []testcontainers.ContainerCustomizer{
		tcpostgres.WithDatabase(database),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(90 * time.Second)),
	}
	if logAll {
		opts = append(opts, testcontainers.CustomizeRequestOption(func(req *testcontainers.GenericContainerRequest) error {
			req.Cmd = []string{"postgres", "-c", "log_statement=all"}
			return nil
		}))
	}
	c, err := tcpostgres.Run(ctx, "postgres:16", opts...)
	if err != nil {
		return nil, fmt.Errorf("bringing up PostgreSQL 16: %w", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, err
	}
	return &Cluster{c: c, host: host, port: port.Port(), User: user, Password: password, Initial: database}, nil
}

// Stop terminates the container.
func (cl *Cluster) Stop() {
	if cl != nil && cl.c != nil {
		_ = cl.c.Terminate(context.Background())
	}
}

// DSN builds a connection string; forImage swaps in the docker host alias.
func (cl *Cluster) DSN(database, user, password string, forImage bool) string {
	host := cl.host
	if forImage {
		host = "host.docker.internal"
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(user, password), Host: host + ":" + cl.port, Path: "/" + database, RawQuery: "sslmode=disable"}
	return u.String()
}

// Admin connects as the cluster superuser.
func (cl *Cluster) Admin(ctx context.Context, database string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, cl.DSN(database, cl.User, cl.Password, false))
}

// Exec runs statements as the superuser in a database.
func (cl *Cluster) Exec(ctx context.Context, database string, statements ...string) error {
	conn, err := cl.Admin(ctx, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	for _, s := range statements {
		if _, err := conn.Exec(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", firstLine(s), err)
		}
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ServerLog returns everything the server has logged.
func (cl *Cluster) ServerLog(ctx context.Context) string {
	r, err := cl.c.Logs(ctx)
	if err != nil {
		return ""
	}
	defer r.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

// LoginRefused reports whether a login is refused by authentication (as
// opposed to failing for an infrastructure reason).
func (cl *Cluster) LoginAccepted(ctx context.Context, database, role, password string) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := pgx.Connect(cctx, cl.DSN(database, role, password, false))
	if err == nil {
		conn.Close(context.Background())
		return true, nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "28P01" || pg.Code == "28000" || pg.Code == "3D000") {
		return false, nil
	}
	if strings.Contains(err.Error(), "SQLSTATE 28") || strings.Contains(err.Error(), "password authentication failed") {
		return false, nil
	}
	return false, fmt.Errorf("login probe failed for infrastructure reasons: %w", err)
}

// ---- catalog snapshot (state-delta universe) --------------------------------

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// Snapshot captures the port-exposed catalog universe: the app role, every
// other role, the database list, and per tracked database its schema version,
// dirty flag, table list, ACL, entry count, imbalance and balances.
func (cl *Cluster) Snapshot(ctx context.Context, tracked ...DatabaseName) (statedelta.Snapshot, error) {
	snap := statedelta.Snapshot{}
	conn, err := cl.Admin(ctx, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	var login bool
	var pwd, marker string
	err = conn.QueryRow(ctx, `SELECT rolcanlogin, coalesce(rolpassword,''), coalesce(shobj_description(oid,'pg_authid'),'')
	                           FROM pg_authid WHERE rolname=$1`, appRole).Scan(&login, &pwd, &marker)
	snap["role.ledgerops_app.exists"] = err == nil
	snap["role.ledgerops_app.can_login"] = err == nil && login
	snap["role.ledgerops_app.password_fingerprint"] = fingerprint(pwd)
	snap["role.ledgerops_app.owner_marker"] = marker

	var others string
	if err := conn.QueryRow(ctx, `SELECT coalesce(string_agg(rolname||rolcanlogin||rolsuper||rolcreaterole||coalesce(rolpassword,''), ',' ORDER BY rolname),'')
	                               FROM pg_authid WHERE rolname <> $1`, appRole).Scan(&others); err != nil {
		return nil, err
	}
	snap["roles.others.fingerprint"] = fingerprint(others)

	rows, err := conn.Query(ctx, `SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	snap["databases"] = strings.Join(names, ",")

	for _, db := range tracked {
		prefix := "db." + string(db) + "."
		var acl string
		_ = conn.QueryRow(ctx, `SELECT coalesce(datacl::text,'') FROM pg_database WHERE datname=$1`, string(db)).Scan(&acl)
		snap[prefix+"acl"] = acl
		dbc, err := cl.Admin(ctx, string(db))
		if err != nil {
			for _, k := range []string{"schema_version", "dirty", "tables", "entries.count", "trial_balance.imbalance_minor", "balances"} {
				snap[prefix+k] = "absent"
			}
			continue
		}
		var hasMig bool
		_ = dbc.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&hasMig)
		version, dirty := 0, false
		if hasMig {
			var v int64
			if err := dbc.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&v, &dirty); err == nil {
				version = int(v)
			}
		}
		snap[prefix+"schema_version"] = version
		snap[prefix+"dirty"] = dirty
		var tables string
		_ = dbc.QueryRow(ctx, `SELECT coalesce(string_agg(tablename, ',' ORDER BY tablename),'') FROM pg_tables WHERE schemaname='public'`).Scan(&tables)
		snap[prefix+"tables"] = tables
		var hasEntries bool
		_ = dbc.QueryRow(ctx, `SELECT to_regclass('public.entries') IS NOT NULL`).Scan(&hasEntries)
		var count, imbalance int64
		balances := ""
		if hasEntries {
			_ = dbc.QueryRow(ctx, `SELECT count(*), coalesce(sum(amount_minor),0) FROM entries`).Scan(&count, &imbalance)
			_ = dbc.QueryRow(ctx, `SELECT coalesce(string_agg(id||':'||balance_minor, ',' ORDER BY id),'') FROM accounts`).Scan(&balances)
		}
		snap[prefix+"entries.count"] = int(count)
		snap[prefix+"trial_balance.imbalance_minor"] = int(imbalance)
		snap[prefix+"balances"] = balances
		dbc.Close(ctx)
	}
	return snap, nil
}

// SnapshotUniverse lists the universe keys for the tracked databases.
func SnapshotUniverse(tracked ...DatabaseName) []string {
	u := []string{
		"role.ledgerops_app.exists", "role.ledgerops_app.can_login",
		"role.ledgerops_app.password_fingerprint", "role.ledgerops_app.owner_marker",
		"roles.others.fingerprint", "databases",
	}
	for _, db := range tracked {
		p := "db." + string(db) + "."
		for _, k := range []string{"acl", "schema_version", "dirty", "tables", "entries.count", "trial_balance.imbalance_minor", "balances"} {
			u = append(u, p+k)
		}
	}
	return u
}

// ---- recording TB so statedelta.AssertStateDelta can be used in godog steps ----

// recordingTB satisfies testing.TB by embedding a nil interface and overriding
// the only two methods AssertStateDelta calls. godog runs from TestMain, so no
// *testing.T exists (the reason earlier suites skipped the helper).
type recordingTB struct {
	testingTB
	message string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
}

// ---- predicates beyond the shared library -----------------------------------

type funcPredicate struct {
	describe string
	check    func(before, after any) (bool, string)
}

func (p funcPredicate) Check(b, a any) (bool, string) { return p.check(b, a) }
func (p funcPredicate) Describe() string              { return p.describe }

// Changed asserts the value differs from before.
func Changed() statedelta.Predicate {
	return funcPredicate{"changed", func(b, a any) (bool, string) {
		if fmt.Sprint(b) != fmt.Sprint(a) {
			return true, ""
		}
		return false, fmt.Sprintf("expected a change from %v", b)
	}}
}

// ContainsText asserts the after value (a string) contains the text.
func ContainsText(text string) statedelta.Predicate {
	return funcPredicate{"containing " + text, func(_, a any) (bool, string) {
		if strings.Contains(fmt.Sprint(a), text) {
			return true, ""
		}
		return false, fmt.Sprintf("%v does not contain %q", a, text)
	}}
}

// ---- fixtures ---------------------------------------------------------------

// ApplyFrozen applies the frozen pre-feature migrations up to version as the
// legacy superuser, in a database named "ledgerops" (what the demo stack did).
func (cl *Cluster) ApplyFrozen(ctx context.Context, version uint) error {
	src, err := iofs.New(prefeature.FS, prefeature.Dir)
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, cl.DSN(legacyDatabase, cl.User, cl.Password, false))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// SeedHistory inserts pairs of balanced entries: half to alice-01, half to
// bob-01, funded from demo-treasury (2*pairs entries, trial balance zero).
func (cl *Cluster) SeedHistory(ctx context.Context, database string, pairs int) error {
	half := pairs / 2
	return cl.Exec(ctx, database,
		fmt.Sprintf(`INSERT INTO accounts(id,kind,balance_minor,currency) VALUES
		  ('demo-treasury','system',%d,'USD'),('alice-01','wallet',%d,'USD'),('bob-01','wallet',%d,'USD')`,
			-100*pairs, 100*half, 100*(pairs-half)),
		fmt.Sprintf(`INSERT INTO transactions(id,recorded_at) SELECT 'txn_seed_'||g, now() FROM generate_series(1,%d) g`, pairs),
		fmt.Sprintf(`INSERT INTO entries(transaction_id,account_id,counterparty_id,amount_minor,currency,recorded_at,sequence)
		  SELECT 'txn_seed_'||g,'demo-treasury',CASE WHEN g<=%[1]d THEN 'alice-01' ELSE 'bob-01' END,-100,'USD',now(),1 FROM generate_series(1,%[2]d) g
		  UNION ALL
		  SELECT 'txn_seed_'||g,CASE WHEN g<=%[1]d THEN 'alice-01' ELSE 'bob-01' END,'demo-treasury',100,'USD',now(),2 FROM generate_series(1,%[2]d) g`, half, pairs))
}

// HoldOn installs an event trigger that makes a DDL statement sleep, so a
// run can be stopped deterministically inside a migration file.
func (cl *Cluster) HoldOn(ctx context.Context, database, tag, identity string, seconds int) error {
	return cl.Exec(ctx, database,
		fmt.Sprintf(`CREATE FUNCTION public.hold_ddl() RETURNS event_trigger LANGUAGE plpgsql AS $$
		 DECLARE r record; BEGIN FOR r IN SELECT * FROM pg_event_trigger_ddl_commands() LOOP
		   IF r.object_identity = '%s' THEN PERFORM pg_sleep(%d); END IF; END LOOP; END $$`, identity, seconds),
		fmt.Sprintf(`CREATE EVENT TRIGGER hold_ddl ON ddl_command_end WHEN TAG IN ('%s') EXECUTE FUNCTION public.hold_ddl()`, tag))
}

// FailOn installs an event trigger that raises when a table is created.
func (cl *Cluster) FailOn(ctx context.Context, database, identity string) error {
	return cl.Exec(ctx, database,
		fmt.Sprintf(`CREATE FUNCTION public.fail_ddl() RETURNS event_trigger LANGUAGE plpgsql AS $$
		 DECLARE r record; BEGIN FOR r IN SELECT * FROM pg_event_trigger_ddl_commands() LOOP
		   IF r.object_identity = '%s' THEN RAISE EXCEPTION 'injected failure'; END IF; END LOOP; END $$`, identity),
		`CREATE EVENT TRIGGER fail_ddl ON ddl_command_end WHEN TAG IN ('CREATE TABLE') EXECUTE FUNCTION public.fail_ddl()`)
}

// BlockedBackendPID waits for a backend in the database stuck in a held DDL
// and returns its pid.
func (cl *Cluster) BlockedBackendPID(ctx context.Context, database string, abort func() bool) (int, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && !abort() {
		conn, err := cl.Admin(ctx, "postgres")
		if err != nil {
			return 0, err
		}
		var pid int
		err = conn.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=$1 AND wait_event='PgSleep' LIMIT 1`, database).Scan(&pid)
		conn.Close(ctx)
		if err == nil {
			return pid, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return 0, errors.New("no migration backend was held in the injected DDL within 30s (migrate never reached the migration file)")
}

// TerminateBackend ends a backend.
func (cl *Cluster) TerminateBackend(ctx context.Context, pid int) error {
	return cl.Exec(ctx, "postgres", fmt.Sprintf(`SELECT pg_terminate_backend(%d)`, pid))
}

// ManagedRole creates the non-superuser privileged role with the supported
// minimum, minus one gap. upgrade selects the legacy-database variant.
func (cl *Cluster) ManagedRole(ctx context.Context, database string, gap *PrivilegeGap, upgrade bool) (user, password string, err error) {
	user, password = "managed_admin", "managed-secret"
	createRole := "CREATEROLE"
	if gap != nil && *gap == MissingCreateRole {
		createRole = "NOCREATEROLE"
	}
	steps := []string{fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s' NOSUPERUSER %s`, user, password, createRole)}
	if gap == nil || *gap != NotDatabaseOwner {
		steps = append(steps, fmt.Sprintf(`ALTER DATABASE "%s" OWNER TO %s`, database, user))
	}
	if err = cl.Exec(ctx, "postgres", steps...); err != nil {
		return
	}
	inDB := []string{}
	if gap != nil && *gap == MissingCreateOnPublic {
		inDB = append(inDB, fmt.Sprintf(`ALTER SCHEMA public OWNER TO %s`, cl.User),
			`REVOKE ALL ON SCHEMA public FROM PUBLIC`,
			fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s`, user))
	}
	if upgrade {
		if gap == nil || *gap != TableOwnedByAnotherRole {
			inDB = append(inDB, fmt.Sprintf(`GRANT %s TO %s`, cl.User, user))
		}
		if gap == nil || *gap != MissingAdminOption {
			inDB = append(inDB, fmt.Sprintf(`GRANT %s TO %s WITH ADMIN OPTION`, appRole, user))
		}
	}
	err = cl.Exec(ctx, database, inDB...)
	return
}

// testingTB is embedded (nil) by recordingTB so it satisfies testing.TB.
type testingTB = testing.TB

// HoldBeforeVersion makes the write that marks migration `version` as running
// sleep inside its own transaction (a trigger on schema_migrations, installed
// by an event trigger as soon as golang-migrate creates the table). Killing
// the run there leaves the previous migration complete and `version` unstarted.
func (cl *Cluster) HoldBeforeVersion(ctx context.Context, database string, version, seconds int) error {
	return cl.Exec(ctx, database,
		fmt.Sprintf(`CREATE FUNCTION public.hold_version() RETURNS trigger LANGUAGE plpgsql AS $$
		 BEGIN IF NEW.version = %d AND NEW.dirty THEN PERFORM pg_sleep(%d); END IF; RETURN NEW; END $$`, version, seconds),
		`CREATE FUNCTION public.arm_hold() RETURNS event_trigger LANGUAGE plpgsql AS $$
		 DECLARE r record; BEGIN FOR r IN SELECT * FROM pg_event_trigger_ddl_commands() LOOP
		   IF r.object_identity = 'public.schema_migrations' THEN
		     EXECUTE 'CREATE TRIGGER hold_version BEFORE INSERT ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.hold_version()';
		   END IF; END LOOP; END $$`,
		`CREATE EVENT TRIGGER arm_hold ON ddl_command_end WHEN TAG IN ('CREATE TABLE') EXECUTE FUNCTION public.arm_hold()`)
}
