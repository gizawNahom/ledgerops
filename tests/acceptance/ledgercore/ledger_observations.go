package ledgercore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"ledgerops/internal/adapters/postgres"
)

// NewLedger builds the composition root with the two fakes the infrastructure
// policy permits, and nothing else faked.
func NewLedger() *Ledger {
	// Default-issued ids use a "seed_" prefix so they can never collide with a
	// hand-picked fixture literal like "txn_0001" pinned via FixNextIdentifier
	// in a .feature file's Given step. Production's real id generator produces
	// an unrelated format again ("txn_1", "txn_2" — no zero-padding), so this
	// fixture-only format was always meant to be test-visible only; the prefix
	// just makes that intent unambiguous instead of a coincidence of today's
	// literals not colliding.
	fixed := []string{"seed_txn_0001", "seed_txn_0002", "seed_txn_0003", "seed_txn_0004", "seed_txn_0005"}
	issued := 0
	return &Ledger{
		operatorKey: "test-operator-key",
		// OPS-13 (step 02-05): resolves to tnt_legacy_seed via
		// legacySeedTenantResolver in ledger_world.go's serve() -- no
		// provisioning call needed, mirroring how operatorKey above is
		// just a fixed literal too.
		tenantKey:   "test-tenant-key",
		actingAs:    ApplicationRole,
		accountKind: map[AccountName]AccountKind{},
		fixedIDs:    fixed,
		clock:       func() time.Time { return time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC) },
		nextID: func() string {
			issued++
			return fmt.Sprintf("seed_txn_%04d", issued)
		},
	}
}

// FixClockAt pins the injected clock. Function-typed port (DDD-13), so the fake
// is a literal — see the infrastructure policy for what it cannot model.
func (l *Ledger) FixClockAt(stamp string) error {
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return fmt.Errorf("clock %q is not an RFC3339 instant: %w", stamp, err)
	}
	l.clock = func() time.Time { return at }
	return nil
}

// FixNextIdentifier pins the next transaction id the generator hands out.
func (l *Ledger) FixNextIdentifier(id string) {
	handed := false
	l.nextID = func() string {
		if !handed {
			handed = true
			return id
		}
		return id + "-next"
	}
}

// --- containers ------------------------------------------------------------
//
// One PostgreSQL 16 instance per test binary, not per scenario. Booting the
// container and migrating from zero costs ~2.5s and now happens once; each
// scenario then gets its own database cloned off the migrated template,
// which is a file copy inside the already-running instance and costs a small
// fraction of that.
//
// Isolation is unchanged, which is what makes this safe: a freshly cloned
// database inherits nothing from a prior scenario, so `contended` and
// `corrupted` still hold structurally and corrupt-04 cannot pass on leftover
// drift. A shared *database* would have broken that; a per-scenario database
// does not. No pooler sits in front of a clone either.
//
// The template is the "ledgerops" database the container is created with,
// and nothing may connect to it after migration: CREATE DATABASE ... TEMPLATE
// is refused while any session is attached. That name is also load-bearing
// in the other direction — migration 0001's GRANT CONNECT names the database
// literally, so "ledgerops" is the only name the migration set may be run
// against, which is why StartWithoutSchema keeps a dedicated container
// rather than cloning.

const templateDatabase = "ledgerops"

var (
	sharedOnce      sync.Once
	sharedContainer testcontainers.Container
	// sharedAdminDSN is privileged and points at the "postgres" maintenance
	// database, since CREATE/DROP DATABASE must be issued from outside the
	// database being created or dropped.
	sharedAdminDSN string
	sharedErr      error

	cloneSeq atomic.Uint64
)

// sharedPostgres boots the instance and migrates the template exactly once
// per test binary, and hands back the maintenance DSN every later clone is
// created through.
func sharedPostgres(ctx context.Context) (string, error) {
	sharedOnce.Do(func() {
		// The application DSN is discarded here: nothing connects to the
		// template as the app role, and every clone derives its own.
		container, _, privilegedDSN, err := startPostgres(ctx)
		if err != nil {
			sharedErr = fmt.Errorf("bringing up PostgreSQL 16: %w", err)
			return
		}
		sharedContainer = container

		if err := postgres.Migrate(ctx, privilegedDSN); err != nil {
			sharedErr = fmt.Errorf("migrating the template from zero: %w", err)
			return
		}
		adminDSN, err := withDatabase(privilegedDSN, "postgres")
		if err != nil {
			sharedErr = err
			return
		}
		// golang-migrate closes its pool on the way out, but CREATE DATABASE
		// ... TEMPLATE fails outright if even one session is still attached,
		// so evict whatever may be lingering rather than racing it.
		if err := evictSessions(ctx, adminDSN, templateDatabase); err != nil {
			sharedErr = err
			return
		}
		sharedAdminDSN = adminDSN
	})
	return sharedAdminDSN, sharedErr
}

// cloneMigratedDatabase gives one scenario its own database, copied from the
// already-migrated template, and returns the application and privileged DSNs
// against it (OPS-10).
func cloneMigratedDatabase(ctx context.Context) (database string, appDSN string, privilegedDSN string, err error) {
	adminDSN, err := sharedPostgres(ctx)
	if err != nil {
		return "", "", "", err
	}

	database = fmt.Sprintf("ledgerops_s%d", cloneSeq.Add(1))
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return "", "", "", fmt.Errorf("connecting to the maintenance database: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, `CREATE DATABASE "`+database+`" TEMPLATE "`+templateDatabase+`"`); err != nil {
		return "", "", "", fmt.Errorf("cloning the migrated template into %q: %w", database, err)
	}
	// Schema and table grants travel inside the copied catalog, but
	// database-level ACLs live in pg_database and are NOT copied by CREATE
	// DATABASE — migration 0001's GRANT CONNECT named the template, so the
	// clone needs its own. Granting it explicitly rather than leaning on
	// PUBLIC's default CONNECT keeps the privilege wall (OPS-10) real here
	// instead of incidental.
	if _, err := conn.Exec(ctx, `GRANT CONNECT ON DATABASE "`+database+`" TO ledgerops_app`); err != nil {
		return "", "", "", fmt.Errorf("granting CONNECT on %q to ledgerops_app: %w", database, err)
	}

	privilegedDSN, err = withDatabase(adminDSN, database)
	if err != nil {
		return "", "", "", err
	}
	appDSN, err = asApplicationRole(privilegedDSN)
	if err != nil {
		return "", "", "", err
	}
	return database, appDSN, privilegedDSN, nil
}

// dropDatabase releases one scenario's database. Best-effort: a clone that
// outlives its scenario costs disk inside a container that is about to be
// terminated anyway, so a failure here must never fail the scenario.
func dropDatabase(database string) {
	if database == "" || sharedAdminDSN == "" {
		return
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, sharedAdminDSN)
	if err != nil {
		return
	}
	defer conn.Close(ctx)
	// WITH (FORCE) (PostgreSQL 13+) evicts any session still attached rather
	// than failing the drop on it.
	_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS "`+database+`" WITH (FORCE)`)
}

// ShutdownPostgres terminates the shared instance. Called from the suite's
// AfterSuite hook — the container outlives every individual scenario. The
// dedicated containers StartWithoutSchema brings up are not its business;
// each scenario's own Close terminates those.
func ShutdownPostgres() {
	if sharedContainer != nil {
		_ = sharedContainer.Terminate(context.Background())
		sharedContainer = nil
	}
}

// evictSessions disconnects everything attached to a database, so it can be
// used as a CREATE DATABASE template or dropped.
func evictSessions(ctx context.Context, adminDSN, database string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connecting to the maintenance database: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		  WHERE datname = $1 AND pid <> pg_backend_pid()`, database); err != nil {
		return fmt.Errorf("evicting sessions from %q: %w", database, err)
	}
	return nil
}

// withDatabase repoints a DSN at another database on the same instance.
func withDatabase(dsn, database string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing the DSN: %w", err)
	}
	parsed.Path = "/" + database
	return parsed.String(), nil
}

// asApplicationRole swaps a privileged DSN's credentials for the
// application role's, which is the only role the service ever connects as
// (OPS-10).
func asApplicationRole(privilegedDSN string) (string, error) {
	parsed, err := url.Parse(privilegedDSN)
	if err != nil {
		return "", fmt.Errorf("parsing the privileged DSN: %w", err)
	}
	parsed.User = url.UserPassword("ledgerops_app", "app-secret")
	return parsed.String(), nil
}

var containerMu sync.Mutex

// startPostgres brings up a DEDICATED, unmigrated PostgreSQL 16 via
// Testcontainers (OPS-11) and returns the application and privileged DSNs
// (OPS-10). Two callers: sharedPostgres above, which migrates the result
// once and turns it into the template every scenario clones; and
// StartWithoutSchema, the one scenario that must run the migration set for
// real against a database named "ledgerops" (see its doc comment).
func startPostgres(ctx context.Context) (container testcontainers.Container, appDSN string, privilegedDSN string, err error) {
	containerMu.Lock()
	defer containerMu.Unlock()

	started, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase(templateDatabase),
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
		return nil, "", "", err
	}
	// Past this point the container exists, so every error path has to
	// release it here — the caller only records the handle on success and
	// would otherwise have nothing left to terminate it with.
	privilegedDSN, err = started.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = started.Terminate(context.Background())
		return nil, "", "", err
	}
	parsed, err := url.Parse(privilegedDSN)
	if err != nil {
		_ = started.Terminate(context.Background())
		return nil, "", "", err
	}
	parsed.User = url.UserPassword("ledgerops_app", "app-secret")
	return started, parsed.String(), privilegedDSN, nil
}

// --- reads through the driving ports --------------------------------------

// readBalance is a Then-side verification read. It authenticates as the
// legitimate operator explicitly rather than inheriting l.actingAs, because a
// preceding When step may have deliberately poisoned actingAs to prove a
// refusal (e.g. "the caller presents no operator key") — the follow-on
// balance check must still be able to observe that nothing moved. This does
// not apply to readTrace/readBooks: those are also invoked as When-side
// actions (Trace, AskWhetherBooksBalance) where inheriting a bad actingAs is
// the scenario under test (see milestone-04/milestone-05 "unidentified
// caller" scenarios).
func (l *Ledger) readBalance(ctx context.Context, account AccountName) (Money, error) {
	answer, err := l.callAsTenant(ctx, ApplicationRole, http.MethodGet, "/accounts/"+url.PathEscape(string(account)), nil, NoIdempotencyKey)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Balance string `json:"balance"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return 0, fmt.Errorf("reading the balance of %q: %w (answer was %s)", account, err, answer.Raw)
	}
	return ParseMoney(payload.Balance), nil
}

func (l *Ledger) readBooks(ctx context.Context, surface Surface) (BooksReport, error) {
	path := "/health/trial-balance"
	if surface == ConsoleSurface {
		path = "/console/verdict"
	}
	answer, err := l.call(ctx, http.MethodGet, path, nil, NoIdempotencyKey)
	if err != nil {
		return BooksReport{}, err
	}
	if answer.Outcome == Refused {
		l.lastAnswer = answer
		return BooksReport{}, nil
	}
	var payload struct {
		Verdict         string `json:"verdict"`
		ImbalanceMinor  int64  `json:"imbalance_minor"`
		EntryCount      int    `json:"entry_count"`
		ElapsedMillis   int    `json:"elapsed_ms"`
		VerdictPosition int    `json:"verdict_position"`
		FigurePosition  int    `json:"first_figure_position"`
		Drifted         []struct {
			AccountID string `json:"account_id"`
			Stored    string `json:"stored"`
			Computed  string `json:"computed"`
			Delta     string `json:"delta"`
		} `json:"drifted"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return BooksReport{}, fmt.Errorf("reading the verdict from the %s surface: %w (answer was %s)", surface, err, answer.Raw)
	}
	report := BooksReport{
		Verdict:           Verdict(payload.Verdict),
		TrialBalance:      Money(payload.ImbalanceMinor),
		EntryCount:        payload.EntryCount,
		ElapsedMillis:     payload.ElapsedMillis,
		VerdictStatedAt:   payload.VerdictPosition,
		FirstFigureStated: payload.FigurePosition,
	}
	for _, row := range payload.Drifted {
		report.Drifted = append(report.Drifted, DriftRow{
			Account:  AccountName(row.AccountID),
			Stored:   ParseMoney(row.Stored),
			Computed: ParseMoney(row.Computed),
			Delta:    ParseMoney(row.Delta),
		})
	}
	return report, nil
}

func (l *Ledger) readTrace(ctx context.Context, account AccountName) ([]TraceRow, error) {
	answer, err := l.call(ctx, http.MethodGet,
		"/accounts/"+url.PathEscape(string(account))+"/entries", nil, NoIdempotencyKey)
	if err != nil {
		return nil, err
	}
	if answer.Outcome == Refused {
		l.lastAnswer = answer
		return nil, nil
	}
	var payload struct {
		Entries []struct {
			TransactionID  string `json:"transaction_id"`
			Counterparty   string `json:"counterparty"`
			Amount         string `json:"amount"`
			RecordedAt     string `json:"recorded_at"`
			RunningBalance string `json:"running_balance"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return nil, fmt.Errorf("tracing %q: %w (answer was %s)", account, err, answer.Raw)
	}
	rows := make([]TraceRow, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		rows = append(rows, TraceRow{
			TransactionID:  entry.TransactionID,
			Counterparty:   AccountName(entry.Counterparty),
			Amount:         ParseMoney(entry.Amount),
			RecordedAt:     entry.RecordedAt,
			RunningBalance: ParseMoney(entry.RunningBalance),
		})
	}
	return rows, nil
}

func decodeAnswer(status int, raw []byte) Answer {
	answer := Answer{Raw: string(raw), Status: status}
	var payload struct {
		TransactionID string `json:"transaction_id"`
		Status        string `json:"status"`
		Error         string `json:"error"`
		AccountID     string `json:"account_id"`
		Available     string `json:"available"`
		Requested     string `json:"requested"`
		Legs          []struct {
			Account string `json:"account"`
			Amount  string `json:"amount"`
		} `json:"legs"`
	}
	_ = json.Unmarshal(raw, &payload)

	switch {
	case status >= 400:
		answer.Outcome = Refused
		answer.Refusal = RefusalKind(payload.Error)
		answer.NamedAccount = AccountName(payload.AccountID)
		if payload.Available != "" {
			answer.Available = ParseMoney(payload.Available)
		}
		if payload.Requested != "" {
			answer.Requested = ParseMoney(payload.Requested)
		}
	case status == http.StatusOK:
		answer.Outcome = Replayed
	default:
		answer.Outcome = Accepted
	}
	answer.TransactionID = payload.TransactionID
	for _, leg := range payload.Legs {
		answer.Legs = append(answer.Legs, Leg{
			Account: AccountName(leg.Account),
			Amount:  ParseMoney(leg.Amount),
		})
	}
	return answer
}

// --- contended runs --------------------------------------------------------

// RaceSpenders fires n simultaneous transfers of the same amount and reports
// the denominators alongside the results. Both are asserted, because a harness
// that quietly ran ten iterations reports zero negatives and passes
// (kpi-contracts.yaml, KPI-2).
func (l *Ledger) RaceSpenders(ctx context.Context, n int, transfer Transfer, distinctKeys bool) error {
	report := RaceReport{}
	answers := make([]Answer, n)
	var wg sync.WaitGroup
	var mu sync.Mutex

	release := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := transfer.Key
			if distinctKeys {
				key = IdempotencyKey(fmt.Sprintf("%s-%d", transfer.Key, i))
			}
			<-release
			answer, err := l.callTenant(ctx, http.MethodPost, "/transfers", map[string]any{
				"from":   string(transfer.From),
				"to":     string(transfer.To),
				"amount": transfer.Amount.String(),
			}, key)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				answers[i] = answer
			}
		}(i)
	}
	close(release)
	wg.Wait()

	distinct := map[string]bool{}
	for _, answer := range answers {
		report.Attempts++
		report.Answered++
		switch answer.Outcome {
		case Accepted, Replayed:
			report.Accepted++
			distinct[answer.TransactionID] = true
		case Refused:
			if answer.Refusal == InsufficientFunds {
				report.RefusedInsufficientFunds++
			}
			if answer.Refusal == "deadlock_detected" {
				report.Deadlocked++
			}
		}
	}
	report.DistinctTransactionIDs = len(distinct)
	pairs, err := postgres.CountEntryPairsForKey(ctx, l.appDSN, string(transfer.Key))
	if err == nil {
		report.EntryPairsStored = pairs
	}
	report.NegativeBalanceObservations, _ = postgres.CountNegativeWalletObservations(ctx, l.appDSN)
	l.lastRace = report
	l.lastAnswers = answers
	return nil
}

// RaceOpposingTransfers fires n movements each way between the same pair of
// accounts, which is the shape that deadlocks when lock ordering is wrong
// (DDD-6).
func (l *Ledger) RaceOpposingTransfers(ctx context.Context, n int, a, b AccountName, amount Money) error {
	if err := l.RaceSpenders(ctx, n, Transfer{From: a, To: b, Amount: amount, Key: "opposing-ab"}, true); err != nil {
		return err
	}
	forward := l.lastRace
	if err := l.RaceSpenders(ctx, n, Transfer{From: b, To: a, Amount: amount, Key: "opposing-ba"}, true); err != nil {
		return err
	}
	l.lastRace = RaceReport{
		Attempts:   forward.Attempts + l.lastRace.Attempts,
		Answered:   forward.Answered + l.lastRace.Answered,
		Accepted:   forward.Accepted + l.lastRace.Accepted,
		Deadlocked: forward.Deadlocked + l.lastRace.Deadlocked,
	}
	return nil
}
