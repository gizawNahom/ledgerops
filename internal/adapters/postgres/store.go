// Package postgres is the driven adapter. SQL stays visible — no ORM —
// because the locking is the part that matters and it must be readable
// (DDD-6).
//
// Migrate, MigrateStep, MigrationStatements, and Open are real as of step
// 01-02 (Migration 0): the schema, the two-role privilege split (OPS-10), and
// the append-only protection on entries (D7) are all live SQL, not scaffold
// no-ops. Begin, and the AccountRepository, TransactionRepository, and
// IdempotencyStore it hands out, are real as of step 01-03, sharing one
// *pgx.Tx per unit of work (DDD-13). InterruptPostingMidWrite is real as of
// step 02-03: it opens its own connection, writes the transaction row and
// one leg, and abandons the transaction unsent, which is how the chaos
// scenario proves atomicity survives a mid-write interruption.
// AttemptOutOfBandChange is real as of step 02-04: it enforces D7 twice
// over, same as migration 0's own SQL, by attempting the out-of-band change
// through a fresh connection under the given role's credentials.
// CountNegativeWalletObservations is real as of step 02-04's corruption
// harness. CountEntryPairsForKey is real as of step 04-03: it resolves a
// claimed key's transaction id and halves the entry count recorded against
// it.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
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

// MigrateStep applies the newest schema change over existing history, which
// is how a scenario exercises a migration running over history it may not
// rewrite.
//
// This brings the store to head via the same idempotent, expand-only m.Up()
// that Migrate uses, rather than advancing exactly one version with
// m.Steps(1). golang-migrate's Steps(1) seeks a specific next version and
// errors ("file does not exist") when none is pending, which is exactly the
// case a scenario built from MigrateFromZero already sits in: the newest
// schema change was already applied while building the populated history it
// is now asked to run "over". Modeling this as "bring current" rather than
// "advance exactly one version" makes re-running the full set over existing
// history — proving nothing in it erases an entry — the actual contract
// under test, and it stays correct if a genuinely pending migration exists
// too: m.Up() applies every migration not yet applied, one included.
func MigrateStep(ctx context.Context, privilegedDSN string) error {
	m, err := newMigrator(privilegedDSN)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
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
// D7 is enforced twice over (migration 0's comment): the app role holds no
// UPDATE/DELETE grant on entries, and an unconditional BEFORE UPDATE OR
// DELETE trigger refuses either statement regardless of role. Only a role
// that can also ALTER TABLE ... DISABLE TRIGGER — the privileged role alone —
// can get an alteration past the second layer, and only by disabling the
// trigger first. That is why "alter_entry" (privileged, used by slice 04's
// CorruptEntry) disables the trigger before altering and re-enables it after,
// while "alter_entry_protection_on" deliberately never touches the trigger,
// so the same privileged connection proves the trigger alone still refuses
// an alteration nobody disabled first.
//
// An error return means the store refused. A nil return means it did not.
func AttemptOutOfBandChange(ctx context.Context, dsn string, action string, args map[string]any) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to attempt an out-of-band change: %w", err)
	}
	defer conn.Close(context.Background())

	switch action {
	case "alter_entry":
		return alterEntryOutOfBand(ctx, conn, args, true)
	case "alter_entry_protection_on":
		return alterEntryOutOfBand(ctx, conn, args, false)
	case "erase_entry":
		return eraseEntryOutOfBand(ctx, conn, args)
	case "disable_protection":
		return disableProtectionOutOfBand(ctx, conn)
	case "alter_stored_balance":
		return alterStoredBalanceOutOfBand(ctx, conn, args)
	default:
		return fmt.Errorf("unknown out-of-band action %q", action)
	}
}

// targetEntry resolves which recorded entry an out-of-band attempt reaches
// for. Most scenarios don't care which entry — they only care whether the
// attempt is refused — so args may be nil and this picks the first recorded
// entry, ordered deterministically. Slice 04's CorruptEntry cares which one,
// so it passes account_id/ordinal explicitly.
func targetEntry(ctx context.Context, conn *pgx.Conn, args map[string]any) (transactionID string, sequence int64, err error) {
	if accountID, ok := args["account_id"].(string); ok {
		ordinal, _ := args["ordinal"].(int)
		row := conn.QueryRow(ctx,
			`SELECT transaction_id, sequence FROM entries WHERE account_id = $1
			 ORDER BY transaction_id, sequence OFFSET $2 LIMIT 1`,
			accountID, ordinal)
		if err := row.Scan(&transactionID, &sequence); err != nil {
			return "", 0, fmt.Errorf("locating the entry to tamper with: %w", err)
		}
		return transactionID, sequence, nil
	}

	row := conn.QueryRow(ctx,
		`SELECT transaction_id, sequence FROM entries ORDER BY transaction_id, sequence LIMIT 1`)
	if err := row.Scan(&transactionID, &sequence); err != nil {
		return "", 0, fmt.Errorf("locating an entry to tamper with: %w", err)
	}
	return transactionID, sequence, nil
}

// alterEntryOutOfBand attempts to change one recorded entry's amount.
// disableTriggerFirst selects between the two contract-shapes AlterEntry and
// AlterEntryProtectedOn need: with it true, the trigger is disabled (which
// only the privileged role's ALTER TABLE grant can do) before the UPDATE and
// re-enabled after, restoring D7 for whatever runs next; with it false, the
// UPDATE runs directly, at the mercy of whatever protection is currently
// live — which is precisely what "with the protection left on" asserts.
func alterEntryOutOfBand(ctx context.Context, conn *pgx.Conn, args map[string]any, disableTriggerFirst bool) error {
	transactionID, sequence, err := targetEntry(ctx, conn, args)
	if err != nil {
		return err
	}

	delta := int64(1)
	if raw, ok := args["delta"].(int64); ok {
		delta = raw
	}

	if disableTriggerFirst {
		if _, err := conn.Exec(ctx, `ALTER TABLE entries DISABLE TRIGGER entries_append_only`); err != nil {
			return fmt.Errorf("disabling the append-only trigger: %w", err)
		}
		defer conn.Exec(context.Background(), `ALTER TABLE entries ENABLE TRIGGER entries_append_only`)
	}

	if _, err := conn.Exec(ctx,
		`UPDATE entries SET amount_minor = amount_minor + $1 WHERE transaction_id = $2 AND sequence = $3`,
		delta, transactionID, sequence); err != nil {
		return fmt.Errorf("altering a recorded entry: %w", err)
	}
	return nil
}

// eraseEntryOutOfBand attempts to delete one recorded entry. Reachable only
// by a role that can both disable the trigger and hold DELETE — i.e. the
// privileged role; the application role is refused at the trigger, the
// privilege grant, or both.
func eraseEntryOutOfBand(ctx context.Context, conn *pgx.Conn, args map[string]any) error {
	transactionID, sequence, err := targetEntry(ctx, conn, args)
	if err != nil {
		return err
	}

	if _, err := conn.Exec(ctx, `ALTER TABLE entries DISABLE TRIGGER entries_append_only`); err != nil {
		return fmt.Errorf("disabling the append-only trigger: %w", err)
	}
	defer conn.Exec(context.Background(), `ALTER TABLE entries ENABLE TRIGGER entries_append_only`)

	if _, err := conn.Exec(ctx,
		`DELETE FROM entries WHERE transaction_id = $1 AND sequence = $2`,
		transactionID, sequence); err != nil {
		return fmt.Errorf("erasing a recorded entry: %w", err)
	}
	return nil
}

// disableProtectionOutOfBand attempts to switch the append-only trigger off
// and, deliberately, never turns it back on within this call — the scenario
// under test is whether the switch-off itself is refused, not what happens
// after.
func disableProtectionOutOfBand(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, `ALTER TABLE entries DISABLE TRIGGER entries_append_only`); err != nil {
		return fmt.Errorf("disabling the append-only trigger: %w", err)
	}
	return nil
}

// alterStoredBalanceOutOfBand attempts to change one account's stored
// balance directly. Accounts carry no append-only guarantee (ADR-003), so
// this is reachable by any role holding UPDATE on accounts — it exists for
// slice 04's drift-detection demo, not for a D7 refusal assertion.
func alterStoredBalanceOutOfBand(ctx context.Context, conn *pgx.Conn, args map[string]any) error {
	accountID, _ := args["account_id"].(string)
	delta, _ := args["delta"].(int64)
	if accountID == "" {
		return fmt.Errorf("alter_stored_balance requires an account_id")
	}
	if _, err := conn.Exec(ctx,
		`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2`,
		delta, accountID); err != nil {
		return fmt.Errorf("altering a stored balance: %w", err)
	}
	return nil
}

// InterruptPostingMidWrite simulates killing the process partway through a
// posting, so the chaos scenario can observe what survived. It writes the
// transaction row and the first leg of the movement — exactly as far as a
// real posting gets before the second leg and COMMIT — inside one real
// PostgreSQL transaction, then abandons that transaction without either
// committing or rolling it back: the raw connection is closed underneath it.
//
// That abandonment is the interruption. A real `kill -9` on the service
// process affords no opportunity to run deferred cleanup either; whatever a
// live transaction never reached COMMIT for, PostgreSQL discards on its own
// when the backend connection drops. No application-level two-phase-commit
// or compensation logic is added here — the database's own atomicity is the
// entire mechanism (BEGIN...COMMIT already guarantees "all or nothing";
// this function proves it by stopping short of COMMIT).
func InterruptPostingMidWrite(ctx context.Context, appDSN, from, to string, amountMinor int64, key string) error {
	conn, err := pgx.Connect(ctx, appDSN)
	if err != nil {
		return fmt.Errorf("connecting to interrupt a posting: %w", err)
	}
	// No Close via defer's normal path is enough on its own — closing a
	// connection that never committed is exactly what happens when the
	// process holding it is killed. The transaction started below is
	// abandoned, not rolled back on purpose: PostgreSQL treats an abandoned
	// backend connection the same way it treats a killed one.
	defer conn.Close(context.Background())

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening the transaction to interrupt: %w", err)
	}

	transactionID := fmt.Sprintf("interrupted-%s-%s-%s", from, to, key)
	recordedAt := time.Now().UTC()

	if _, err := tx.Exec(ctx,
		`INSERT INTO transactions (id, recorded_at) VALUES ($1, $2)`,
		transactionID, recordedAt); err != nil {
		return fmt.Errorf("writing the transaction row before interruption: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO entries (transaction_id, account_id, counterparty_id, amount_minor, currency, recorded_at, sequence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		transactionID, from, to, -amountMinor, "USD", recordedAt, 0); err != nil {
		return fmt.Errorf("writing the first leg before interruption: %w", err)
	}

	// The interruption happens here: the second leg is never written, and
	// neither Commit nor Rollback is ever called on tx. The deferred
	// conn.Close above abandons it, which is what a process kill would have
	// left behind for PostgreSQL to discard.
	return nil
}

// CountEntryPairsForKey counts the entry pairs recorded under one idempotency
// key. Asserted alongside the distinct transaction count because the id check
// alone would miss a double write that happens to render the same id
// (kpi-contracts.yaml, KPI-3).
//
// It resolves the key's claimed transaction id (idempotency_keys, migration
// 0002) and counts the entries recorded against that transaction, halved: a
// posting always writes exactly two legs (DDD-8), so entry count / 2 is the
// pair count. A key that was never claimed resolves the subquery to no row,
// which makes the outer WHERE compare against NULL and count zero — the
// correct answer for "no transaction exists under that key yet".
func CountEntryPairsForKey(ctx context.Context, appDSN, key string) (int, error) {
	conn, err := pgx.Connect(ctx, appDSN)
	if err != nil {
		return 0, fmt.Errorf("connecting to count entry pairs for key %q: %w", key, err)
	}
	defer conn.Close(context.Background())

	var entryCount int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM entries
		 WHERE transaction_id = (SELECT transaction_id FROM idempotency_keys WHERE key = $1)`,
		key,
	).Scan(&entryCount); err != nil {
		return 0, fmt.Errorf("counting entry pairs for key %q: %w", key, err)
	}
	return entryCount / 2, nil
}

// CountNegativeWalletObservations reports how many wallet accounts carry a
// stored balance below zero at the moment this is called.
//
// A single post-race read is sufficient to stand for "throughout the run,"
// not just "at the end": AccountRepository.LockForUpdate acquires each
// touched account's row lock inside the same *pgx.Tx that
// AccountRepository.ApplyDeltas later writes through and that transaction's
// COMMIT closes (DDD-6). No other transaction can see a wallet's balance
// mid-update — the row lock is held for the tx's entire lifetime, not just
// the SELECT — so a wallet's stored value only ever transitions between
// fully-committed states. If any commit had ever applied a delta
// domain.Post should have refused (I4), the row would still show negative
// here; there is no window in which a transient negative dip could occur
// and then heal before this scan runs.
func CountNegativeWalletObservations(ctx context.Context, appDSN string) (int, error) {
	conn, err := pgx.Connect(ctx, appDSN)
	if err != nil {
		return 0, fmt.Errorf("connecting to observe wallet balances: %w", err)
	}
	defer conn.Close(context.Background())

	var count int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM accounts WHERE kind = $1 AND balance_minor < 0`,
		string(domain.Wallet),
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting negative wallet balances: %w", err)
	}
	return count, nil
}
