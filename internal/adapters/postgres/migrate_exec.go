package postgres

// The effect shell of migrate: gathers facts (read-only), probes write
// rights (always rolled back), asks the pure PlanMigration for a verdict, and
// only then executes the plan's steps (ADR-022, DDD-39/42).

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ledgerops/internal/support"
)

// pgErrorInsufficientPrivilege is SQLSTATE 42501.
const pgErrorInsufficientPrivilege = "42501"

// RefusalError is a migrate run refused by the plan before any change.
type RefusalError struct{ Plan Plan }

func (e *RefusalError) Error() string { return "refused: " + e.Plan.Refusal }

// factQueries are the only statements FactReader may run: all SELECT.
const (
	queryDatabaseName  = `SELECT current_database()`
	queryCanCreateRole = `SELECT rolcreaterole OR rolsuper FROM pg_roles WHERE rolname = current_user`
	queryRoleExists    = `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ledgerops_app')`
	queryAdminOption   = `SELECT (SELECT rolsuper FROM pg_roles WHERE rolname = current_user) OR EXISTS (
		SELECT 1 FROM pg_auth_members m
		JOIN pg_roles granted ON granted.oid = m.roleid
		JOIN pg_roles member ON member.oid = m.member
		WHERE granted.rolname = 'ledgerops_app' AND member.rolname = current_user AND m.admin_option)`
	queryOwnsDatabase    = `SELECT has_database_privilege(current_user, current_database(), 'CONNECT WITH GRANT OPTION')`
	queryCreateOnPublic  = `SELECT has_schema_privilege(current_user, 'public', 'CREATE')`
	queryHasLedgerSchema = `SELECT to_regclass('public.schema_migrations') IS NOT NULL`
	queryOwnsTables      = `SELECT NOT EXISTS (
		SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND NOT pg_has_role(current_user, c.relowner, 'USAGE'))`
	queryCanReadVersion = `SELECT has_table_privilege(current_user, 'public.schema_migrations', 'SELECT')`
	queryCurrentVersion = `SELECT version, dirty FROM public.schema_migrations LIMIT 1`
)

var factQueries = []string{
	queryDatabaseName, queryCanCreateRole, queryRoleExists, queryAdminOption,
	queryOwnsDatabase, queryCreateOnPublic, queryHasLedgerSchema, queryOwnsTables, queryCanReadVersion, queryCurrentVersion,
}

// pgFactReader is the FactReader adapter: SELECT-only catalog queries.
type pgFactReader struct {
	ctx           context.Context
	privilegedDSN string
}

var _ FactReader = pgFactReader{}

func (r pgFactReader) ReadFacts(target string) (Facts, error) {
	conn, err := pgx.Connect(r.ctx, r.privilegedDSN)
	if err != nil {
		return Facts{}, fmt.Errorf("connecting to read the preflight facts: %w", err)
	}
	defer func() { _ = conn.Close(r.ctx) }()

	facts := Facts{TargetDatabase: target}
	scalars := []struct {
		query string
		into  *bool
	}{
		{queryCanCreateRole, &facts.CanCreateRole},
		{queryRoleExists, &facts.RoleExists},
		{queryAdminOption, &facts.HasAdminOptionOnRole},
		{queryOwnsDatabase, &facts.OwnsDatabaseOrGrant},
		{queryCreateOnPublic, &facts.CanCreateOnPublic},
		{queryHasLedgerSchema, &facts.HasLedgerSchema},
		{queryOwnsTables, &facts.OwnsLedgerTables},
	}
	for _, scalar := range scalars {
		if err := conn.QueryRow(r.ctx, scalar.query).Scan(scalar.into); err != nil {
			return Facts{}, fmt.Errorf("reading the preflight facts: %w", err)
		}
	}
	if facts.TargetDatabase == "" {
		if err := conn.QueryRow(r.ctx, queryDatabaseName).Scan(&facts.TargetDatabase); err != nil {
			return Facts{}, fmt.Errorf("reading the database name: %w", err)
		}
	}
	current, dirty, err := readCurrentVersion(r.ctx, conn, facts.HasLedgerSchema)
	if err != nil {
		return Facts{}, err
	}
	if dirty {
		facts.DirtyVersion = &current
	}
	facts.PendingVersions = pendingVersionsAfter(current)
	return facts, nil
}

func readCurrentVersion(ctx context.Context, conn *pgx.Conn, hasSchema bool) (uint, bool, error) {
	if !hasSchema {
		return 0, false, nil
	}
	// A history table the role cannot read leaves the version unknown: every
	// migration counts as pending, which is the conservative reading.
	var canRead bool
	if err := conn.QueryRow(ctx, queryCanReadVersion).Scan(&canRead); err != nil {
		return 0, false, fmt.Errorf("reading the schema version: %w", err)
	}
	if !canRead {
		return 0, false, nil
	}
	var version int64
	var dirty bool
	err := conn.QueryRow(ctx, queryCurrentVersion).Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reading the schema version: %w", err)
	}
	return uint(version), dirty, nil
}

// pendingVersionsAfter lists the embedded migration versions above current.
func pendingVersionsAfter(current uint) []uint {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil
	}
	var pending []uint
	for _, entry := range entries {
		prefix, _, found := strings.Cut(entry.Name(), "_")
		if !found || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		if version, err := strconv.ParseUint(prefix, 10, 32); err == nil && uint(version) > current {
			pending = append(pending, uint(version))
		}
	}
	return pending
}

// pgPreflightProber is the PreflightProber adapter. It is explicitly
// write-capable: each probe runs a real DDL statement inside a transaction
// that is ALWAYS rolled back, never committed.
type pgPreflightProber struct {
	ctx           context.Context
	privilegedDSN string
}

var _ PreflightProber = pgPreflightProber{}

const (
	probeCreateTable = `CREATE TABLE public.ledgerops_preflight_probe (probe int)`
	probeCreateRole  = `CREATE ROLE ledgerops_preflight_probe NOLOGIN`
)

// Probe reports only the fields it observes: CanCreateRole, CanCreateOnPublic.
func (p pgPreflightProber) Probe(target string) (Facts, error) {
	conn, err := pgx.Connect(p.ctx, p.privilegedDSN)
	if err != nil {
		return Facts{}, fmt.Errorf("connecting to probe the privileges: %w", err)
	}
	defer func() { _ = conn.Close(p.ctx) }()

	canCreateOnPublic, err := probeSucceeds(p.ctx, conn, probeCreateTable)
	if err != nil {
		return Facts{}, err
	}
	canCreateRole, err := probeSucceeds(p.ctx, conn, probeCreateRole)
	if err != nil {
		return Facts{}, err
	}
	return Facts{TargetDatabase: target, CanCreateOnPublic: canCreateOnPublic, CanCreateRole: canCreateRole}, nil
}

// probeSucceeds runs statement in a transaction it always rolls back. A
// permission error is an answer (false); any other error is a failure.
func probeSucceeds(ctx context.Context, conn *pgx.Conn, statement string) (bool, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("starting a privilege probe: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, statement)
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgErrorInsufficientPrivilege {
		return false, nil
	}
	return false, fmt.Errorf("probing a privilege: %w", err)
}

// mergeProbed keeps a right only when both the catalog and the probe grant it.
func mergeProbed(read, probed Facts) Facts {
	read.CanCreateRole = read.CanCreateRole && probed.CanCreateRole
	read.CanCreateOnPublic = read.CanCreateOnPublic && probed.CanCreateOnPublic
	return read
}

// preflight gathers facts and returns the plan for them.
func preflight(ctx context.Context, privilegedDSN string, appPassword support.Secret) (Plan, error) {
	read, err := pgFactReader{ctx: ctx, privilegedDSN: privilegedDSN}.ReadFacts("")
	if err != nil {
		return Plan{}, redactedError(err, appPassword)
	}
	probed, err := pgPreflightProber{ctx: ctx, privilegedDSN: privilegedDSN}.Probe(read.TargetDatabase)
	if err != nil {
		return Plan{}, redactedError(err, appPassword)
	}
	config := MigrateConfig{PrivilegedDSN: support.NewSecret(privilegedDSN), AppPassword: appPassword}
	return PlanMigration(mergeProbed(read, probed), config), nil
}

// executePlan runs the steps of an accepted plan in order. A runtime 42501 is
// a backstop: it says whether schema changes had been applied.
func executePlan(ctx context.Context, plan Plan, privilegedDSN string, appPassword support.Secret, action string) error {
	schemaApplied := false
	for _, step := range plan.Steps {
		var err error
		switch step {
		case ClaimRole:
			err = claimAppRole(ctx, privilegedDSN)
		case ApplySchema:
			err = applySchema(privilegedDSN, action)
			schemaApplied = err == nil || schemaApplied
		case SetAppRolePassword:
			err = setAppRolePassword(ctx, privilegedDSN, appPassword)
		}
		if err != nil {
			return redactedError(annotatePermissionDenied(err, schemaApplied), appPassword)
		}
	}
	return nil
}

func setAppRolePassword(ctx context.Context, privilegedDSN string, appPassword support.Secret) error {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generating the password salt: %w", err)
	}
	verifier := BuildSCRAMVerifier(appPassword, salt, scramIterations)
	return pgRolePasswordSetter{ctx: ctx, privilegedDSN: privilegedDSN}.SetVerifier(verifier)
}

func annotatePermissionDenied(err error, schemaApplied bool) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgErrorInsufficientPrivilege {
		return err
	}
	if schemaApplied {
		return fmt.Errorf("%w (permission denied after schema changes were applied)", err)
	}
	return fmt.Errorf("%w (permission denied; no changes applied)", err)
}
