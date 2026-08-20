// Package postgres is the driven adapter. SQL stays visible — no ORM —
// because the locking is the part that matters and it must be readable
// (DDD-6).
//
// Migrate, MigrateStep, MigrationStatements, and Open are real as of step
// 01-02 (Migration 0): the schema, the two-role privilege split (OPS-10), and
// the append-only protection on entries (D7) are all live SQL, not scaffold
// no-ops. Begin, and the AccountRepository, TransactionRepository, and
// IdempotencyStore it hands out, are real as of step 01-03, sharing one
// *pgx.Tx per unit of work (DDD-13). AttemptOutOfBandChange,
// InterruptPostingMidWrite, CountEntryPairsForKey, and
// CountNegativeWalletObservations remain RED scaffolds — they land with the
// corruption harness later.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerops/internal/app/ports"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// newMigrator loads the embedded migration set and opens it against the
// privileged DSN. DDL runs only as ledgerops_migrate — the service never
// connects as this role (OPS-10).
func newMigrator(privilegedDSN string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("loading embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, privilegedDSN)
	if err != nil {
		return nil, fmt.Errorf("opening migrator as the privileged role: %w", err)
	}
	return m, nil
}

func closeMigrator(m *migrate.Migrate) {
	sourceErr, dbErr := m.Close()
	_ = sourceErr
	_ = dbErr
}

// Migrate applies the whole migration set as the privileged role. Migrations
// are expand-only: no migration may DELETE from or drop the entry table, and
// every schema change must leave the previous binary able to run against the
// new schema (brief.md § Deployment shape).
func Migrate(ctx context.Context, privilegedDSN string) error {
	m, err := newMigrator(privilegedDSN)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrating from zero: %w", err)
	}
	return nil
}

// MigrateStep applies only the newest schema change, which is how a scenario
// exercises a migration running over history it may not rewrite.
func MigrateStep(ctx context.Context, privilegedDSN string) error {
	m, err := newMigrator(privilegedDSN)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Steps(1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying the newest migration: %w", err)
	}
	return nil
}

// MigrationStatements returns every migration's up-direction SQL by file
// name, so a scenario can assert the expand-only rule across the whole set
// rather than only the newest. Down migrations are excluded: they exist for
// local rollback only and are never applied over recorded history, so
// scanning them for D7 violations would flag a legitimate `DROP TABLE
// entries` that only ever runs against an empty database.
func MigrationStatements() (map[string]string, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}

	statements := make(map[string]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		statements[name] = string(contents)
	}
	return statements, nil
}

// Open connects as the application role. That role holds SELECT and INSERT
// on entries and has UPDATE and DELETE revoked (OPS-10) — the service never
// connects as the migrate role.
func Open(ctx context.Context, appDSN string) (ports.Store, error) {
	pool, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		return nil, fmt.Errorf("opening the store as the application role: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("the application role could not reach the store: %w", err)
	}
	return &store{pool: pool}, nil
}

// store is the real ports.Store, backed by a pgx connection pool held under
// the application role's credentials.
type store struct {
	pool *pgxpool.Pool
}

// Begin opens one unit of work as a real pgx transaction. The three
// repositories it hands out (AccountRepository, TransactionRepository,
// IdempotencyStore) all share this same *pgx.Tx handle — that sharing is
// what DDD-13 means by "a caller could wire two of them to different
// transactions": as an interface, UnitOfWork makes that impossible rather
// than merely undocumented.
func (s *store) Begin(ctx context.Context) (ports.UnitOfWork, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening a unit of work: %w", err)
	}
	return &unitOfWork{tx: tx}, nil
}

func (s *store) Close() error {
	s.pool.Close()
	return nil
}

// unitOfWork is the real ports.UnitOfWork. One *pgx.Tx, three repositories
// reading and writing through it, and nothing reachable outside it.
type unitOfWork struct {
	tx pgx.Tx
}

func (u *unitOfWork) Accounts() ports.AccountRepository {
	return accountRepository{tx: u.tx}
}

func (u *unitOfWork) Transactions() ports.TransactionRepository {
	return transactionRepository{tx: u.tx}
}

func (u *unitOfWork) Idempotency() ports.IdempotencyStore {
	return idempotencyStore{tx: u.tx}
}

// Commit and Rollback are idiomatic pgx.Tx passthroughs. A commit or
// rollback attempted twice (e.g. Commit succeeding, then a deferred Rollback
// firing anyway) is left to pgx's own error, which callers treat as
// best-effort cleanup, not a fresh failure.
func (u *unitOfWork) Commit(ctx context.Context) error {
	return u.tx.Commit(ctx)
}

func (u *unitOfWork) Rollback(ctx context.Context) error {
	return u.tx.Rollback(ctx)
}

// AttemptOutOfBandChange reaches past the driving ports to try to rewrite
// recorded history as the given role. It exists for exactly two reasons, both
// of which are acceptance criteria:
//
//   - as the application role, every attempt MUST be refused — that is what
//     makes D7 structural rather than discipline (OPS-10);
//   - as the privileged role, the alteration MUST succeed, because slice 04's
//     verdict only means something if a genuine drift can be produced.
//
// An error return means the store refused. A nil return means it did not.
func AttemptOutOfBandChange(ctx context.Context, dsn string, action string, args map[string]any) error {
	panic("postgres.AttemptOutOfBandChange not yet implemented -- RED scaffold")
}

// InterruptPostingMidWrite kills the process partway through a posting, so the
// chaos scenario can observe what survived. If this turns out to be awkward to
// write, the chaos demo — the thing that makes slice 01 worth anything — will
// not exist (slice-01 § Pre-slice SPIKE).
func InterruptPostingMidWrite(ctx context.Context, appDSN, from, to string, amountMinor int64, key string) error {
	panic("postgres.InterruptPostingMidWrite not yet implemented -- RED scaffold")
}

// CountEntryPairsForKey counts the entry pairs recorded under one idempotency
// key. Asserted alongside the distinct transaction count because the id check
// alone would miss a double write that happens to render the same id
// (kpi-contracts.yaml, KPI-3).
func CountEntryPairsForKey(ctx context.Context, appDSN, key string) (int, error) {
	return 0, fmt.Errorf("postgres.CountEntryPairsForKey not yet implemented -- RED scaffold")
}

// CountNegativeWalletObservations reports how many times a wallet balance was
// seen below zero during a contended run. Observed throughout the run, not
// sampled at the end: a run that dips negative and recovers has still broken
// I4's promise.
func CountNegativeWalletObservations(ctx context.Context, appDSN string) (int, error) {
	return 0, fmt.Errorf("postgres.CountNegativeWalletObservations not yet implemented -- RED scaffold")
}
