package intertenanttransfer

// World is the acceptance suite's composition root (Tier A, Mandate 10). It
// wires the PRODUCTION composition root: the real chi router
// (internal/adapters/http), real requireOperatorKey/requireTenantKey
// middleware, and a real PostgreSQL 16 from Testcontainers (WS strategy C,
// per brief.md § For Acceptance Designer -- "never faked" extended to
// TenantLinkRepository/CounterpartyAliasRepository/TransferStateRepository).
//
// startPostgres/serve/StartAgainstEmptyStore below are DUPLICATED from
// tests/acceptance/multitenancy/world.go rather than imported -- both are
// unexported in that package. This is test-infra bootstrap duplication, not
// a Mandate-12 business-logic violation (multitenancy's own world.go header
// draws the identical line against ledgercore).
//
// Every driving-port call in this file goes over the real HTTP server. Three
// of the five new/extended routes are RED scaffolds today (POST
// /tenant-links, DELETE /tenant-links/{link_id}, POST /counterparties, the
// cross-tenant variant of POST /transfers, GET /transfers/{transfer_id}) --
// see internal/adapters/http/router.go's own scaffold(...) calls. Every
// scenario in this suite is therefore expected to fail at its Then
// assertion today (RED, MISSING_FUNCTIONALITY), never at a Go compile error
// or a panic (Mandate 7).
//
// Fault injection into a specific leg attempt, simulating a process crash at
// either of two DISTINCT windows, and single-stepping the retry ticker are
// wired for real as of step 03-01, over a test-only HTTP seam
// (internal/adapters/http/testonly_faults.go) this World's own serve()
// mounts by passing Deps.EnableTestOnlyFaultSeam: true -- a flag
// cmd/api/main.go never sets, mirroring postgres.AttemptOutOfBandChange's
// own "what the suite may never do for real, except through a named back
// door" precedent (multitenancy/world.go), adapted to this suite's own
// "everything through HTTP" rule (every driving-port call in this file goes
// over the real server, never a direct Go reference into it). The two crash
// windows are semantically different and get separate methods rather than
// one overloaded name (2026-09-07 follow-up fix 2, replacing the earlier
// single SimulateCrashBeforeFirstAttempt that conflated them):
//   - SimulateCrashBeforeForwardLegAttempt: the FORWARD-path window, between
//     Leg 1's commit and Leg 2's first attempt (milestone-03's crash-recovery
//     scenario).
//   - SimulateCrashBeforeReversalAttempt: the REVERSAL-path window, between a
//     later leg's reversal committing and an earlier leg's reversal attempt
//     ever running (milestone-04's own crash scenario, mirroring the forward
//     case on the compensating path). No reversal dispatcher exists yet
//     (step 03-02 onward's own scope) to ever consume this marker -- calling
//     it today registers the intent and nothing observably reacts to it,
//     which is the correct, expected shape until that dispatcher lands.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/adapters/postgres"
	"ledgerops/internal/app/ports"
)

type tenantRecord struct {
	ID  string
	Key string
}

type World struct {
	server *httptest.Server
	client *http.Client

	// database is this scenario's own clone of the migrated template, and
	// store is the pool opened against it. Close drops the one and closes
	// the other; the PostgreSQL instance itself is shared per test binary
	// and outlives every scenario (see § containers).
	database string
	store    ports.Store

	appDSN        string
	privilegedDSN string

	adminKey string

	tenants  map[TenantName]tenantRecord
	actingAs Caller

	// links keyed by canonical (a<b) pair name, for the "unaffected"/"reads
	// revoked" assertions that don't want a second HTTP round trip.
	links map[string]LinkAnswer

	lastLinkAnswer     LinkAnswer
	priorLinkAnswer    LinkAnswer
	lastAliasAnswer    AliasAnswer
	lastTransferAnswer TransferAnswer
	priorTransferID    string
	secondTransferID   string

	// lastTransferFrom/lastTransferTo remember seedSettlingTransfer's own
	// sender/receiver pair -- the milestone-03 back-propagation fix (2026-09-08)
	// needs these to resolve the tnt_platform mirror account naming convention
	// (platformMirrorAccountID) and the sender's own settlement account,
	// neither of which a bare transfer_id alone identifies.
	lastTransferFrom TenantName
	lastTransferTo   TenantName

	// inspectedBalance/inspectedEntries cache the result of a real
	// GET /accounts/{id} or GET /accounts/{id}/entries call an "inspected"
	// When step made, for a following Then step to assert against -- never
	// set by a Given, per Mandate 2.
	inspectedBalance Money
	inspectedEntries []entryWireView

	lastRefusal RefusalKind
	lastStatus  int
}

func NewWorld() *World {
	return &World{
		adminKey: "test-operator-key",
		actingAs: PlatformAdmin(),
		tenants:  map[TenantName]tenantRecord{},
		links:    map[string]LinkAnswer{},
	}
}

// --- lifecycle -------------------------------------------------------------

func (w *World) StartAgainstEmptyStore(ctx context.Context) error {
	database, appDSN, privilegedDSN, err := cloneMigratedDatabase(ctx)
	if err != nil {
		return fmt.Errorf("bringing up PostgreSQL 16: %w", err)
	}
	w.database = database
	w.appDSN, w.privilegedDSN = appDSN, privilegedDSN
	return w.serve(ctx)
}

func (w *World) serve(ctx context.Context) error {
	// Closing any pool a previous serve() opened, so re-serving against the
	// same store replaces the connection pool instead of stacking a second
	// one on top of it.
	if w.store != nil {
		_ = w.store.Close()
		w.store = nil
	}
	store, err := postgres.Open(ctx, w.appDSN)
	if err != nil {
		return fmt.Errorf("opening the store as the application role: %w", err)
	}
	w.store = store
	handler := apphttp.NewRouter(apphttp.Deps{
		Store:             store,
		OperatorKey:       w.adminKey,
		Clock:             time.Now,
		IDGenerator:       func() string { return uuidLike() },
		TenantKeyResolver: postgres.NewTenantKeyResolver(w.appDSN),
		// This suite's own composition root is the ONLY caller that ever
		// sets this true (see internal/adapters/http/router.go and
		// testonly_faults.go) — cmd/api/main.go never does, mirroring
		// postgres.AttemptOutOfBandChange's own back-door precedent.
		EnableTestOnlyFaultSeam: true,
	})
	w.server = httptest.NewServer(handler)
	w.client = w.server.Client()
	return nil
}

func (w *World) EnsureStarted(ctx context.Context) error {
	if w.server != nil {
		return nil
	}
	return w.StartAgainstEmptyStore(ctx)
}

// Stop releases the server but keeps the store, so a scenario can serve
// again against the same PostgreSQL instance. Releasing the container is
// Close's job, not this one's.
func (w *World) Stop() {
	if w.server != nil {
		w.server.Close()
		w.server = nil
	}
}

// Close is the scenario-teardown entry point: it releases the server, closes
// the pool, and drops this scenario's database. The PostgreSQL instance
// itself is shared per test binary and is released by ShutdownPostgres from
// the suite's AfterSuite hook.
//
// Teardown runs on context.Background(), not the scenario's context: a
// failing or timed-out scenario arrives here with a cancelled context, and
// that is precisely when its resources most need releasing.
func (w *World) Close() {
	w.Stop()
	if w.store != nil {
		_ = w.store.Close()
		w.store = nil
	}
	dropDatabase(w.database)
	w.database = ""
}

func (w *World) ActAs(c Caller) { w.actingAs = c }

// --- containers --------------------------------------------------------
//
// One PostgreSQL 16 instance per test binary, not per scenario. Booting the
// container and migrating from zero costs ~2.5s and now happens once; each
// scenario then gets its own database cloned off the migrated template,
// which is a file copy inside the already-running instance and costs a small
// fraction of that.
//
// Isolation is unchanged, which is the point: a freshly cloned database
// inherits nothing from a prior scenario, so every scenario still starts
// from a known-empty store. What a shared *database* would have broken —
// one scenario passing on rows another left behind — a per-scenario
// database does not.
//
// The template is the "ledgerops" database the container is created with,
// and nothing may connect to it after migration: CREATE DATABASE ... TEMPLATE
// is refused while any session is attached. That name is also load-bearing
// in the other direction — migration 0001's GRANT CONNECT names the database
// literally, so "ledgerops" is the only name the migration set may be run
// against.

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
		container, err := tcpostgres.Run(ctx,
			"postgres:16",
			tcpostgres.WithDatabase(templateDatabase),
			tcpostgres.WithUsername("ledgerops_migrate"),
			tcpostgres.WithPassword("migrate-secret"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(90*time.Second),
			),
		)
		if err != nil {
			sharedErr = fmt.Errorf("bringing up PostgreSQL 16: %w", err)
			return
		}
		sharedContainer = container

		templateDSN, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedErr = fmt.Errorf("reading the privileged connection string: %w", err)
			return
		}
		if err := postgres.Migrate(ctx, templateDSN); err != nil {
			sharedErr = fmt.Errorf("migrating the template from zero: %w", err)
			return
		}

		adminDSN, err := withDatabase(templateDSN, "postgres")
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
// AfterSuite hook — the container outlives every individual scenario.
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

var uuidCounter int
var uuidMu sync.Mutex

func uuidLike() string {
	uuidMu.Lock()
	defer uuidMu.Unlock()
	uuidCounter++
	return fmt.Sprintf("itt_%d_%d", time.Now().UnixNano(), uuidCounter)
}

func canonicalPair(a, b TenantName) string {
	if a < b {
		return string(a) + "|" + string(b)
	}
	return string(b) + "|" + string(a)
}

// --- driving-port calls: tenant provisioning (reused, unmodified port) -----

// GivenTenantProvisioned provisions a tenant through the real, unmodified
// POST /tenants port -- this feature introduces no change to it.
func (w *World) GivenTenantProvisioned(ctx context.Context, name TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.ActAs(PlatformAdmin())
	body := map[string]any{"name": string(name)}
	status, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodPost, "/tenants", body)
	if err != nil {
		return err
	}
	var payload struct {
		TenantID  string `json:"tenant_id"`
		TenantKey string `json:"tenant_key"`
	}
	_ = json.Unmarshal(raw, &payload)
	if status == http.StatusCreated {
		w.tenants[name] = tenantRecord{ID: payload.TenantID, Key: payload.TenantKey}
		return nil
	}
	return fmt.Errorf("provisioning tenant %q as a Given-side precondition failed: status=%d body=%s", name, status, raw)
}

// --- driving-port calls: tenant links (slice 01) ----------------------------

func (w *World) AuthorizeTenantPair(ctx context.Context, a, b TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorLinkAnswer = w.lastLinkAnswer
	body := map[string]any{"tenant_a": string(a), "tenant_b": string(b)}
	status, raw, err := w.rawCall(ctx, w.actingAs, http.MethodPost, "/tenant-links", body)
	if err != nil {
		return err
	}
	answer := decodeLinkAnswer(status, raw)
	answer.TenantA, answer.TenantB = string(a), string(b)
	w.lastLinkAnswer = answer
	w.lastRefusal, w.lastStatus = answer.Refusal, answer.Status
	if answer.Created() {
		w.links[canonicalPair(a, b)] = answer
	}
	return nil
}

// GivenPairAuthorized is the Given-side convenience: authorize as the admin,
// leaving w.actingAs at whatever a following When step sets explicitly
// (Mandate 2 -- Given never sets the expected output).
func (w *World) GivenPairAuthorized(ctx context.Context, a, b TenantName) error {
	w.ActAs(PlatformAdmin())
	return w.AuthorizeTenantPair(ctx, a, b)
}

func (w *World) RevokeTenantLink(ctx context.Context, a, b TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	link, known := w.links[canonicalPair(a, b)]
	linkID := link.LinkID
	if !known {
		linkID = "unresolved-link-" + canonicalPair(a, b)
	}
	status, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodDelete, "/tenant-links/"+url.PathEscape(linkID), nil)
	if err != nil {
		return err
	}
	answer := decodeLinkAnswer(status, raw)
	answer.TenantA, answer.TenantB = string(a), string(b)
	if status >= 200 && status < 300 {
		answer.Status_ = LinkRevoked
		answer.LinkID = linkID
	}
	w.links[canonicalPair(a, b)] = answer
	w.lastLinkAnswer = answer
	w.lastRefusal, w.lastStatus = answer.Refusal, answer.Status
	return nil
}

// --- driving-port calls: counterparty aliases (slice 02) -------------------

func (w *World) RegisterAlias(ctx context.Context, as TenantName, alias AliasName, targetTenant TenantName, targetAccount AccountName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	body := map[string]any{
		"alias":             string(alias),
		"target_tenant_id":  w.resolvedIDOrName(targetTenant),
		"target_account_id": string(targetAccount),
	}
	status, raw, err := w.rawCall(ctx, AsTenant(as), http.MethodPost, "/counterparties", body)
	if err != nil {
		return err
	}
	answer := decodeAliasAnswer(status, raw)
	answer.Alias = alias
	w.lastAliasAnswer = answer
	w.lastRefusal, w.lastStatus = answer.Refusal, answer.Status
	return nil
}

func (w *World) resolvedIDOrName(t TenantName) string {
	if record, ok := w.tenants[t]; ok {
		return record.ID
	}
	return string(t)
}

// --- driving-port calls: cross-tenant transfers (slice 02-04) --------------

func (w *World) SendCrossTenantTransfer(ctx context.Context, as TenantName, amount Money, alias AliasName, key IdempotencyKey) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorTransferID = w.lastTransferAnswer.TransferID
	body := map[string]any{
		"counterparty_alias": string(alias),
		"amount":             amount.String(),
	}
	status, raw, err := w.rawCallWithKey(ctx, AsTenant(as), http.MethodPost, "/transfers", body, string(key))
	if err != nil {
		return err
	}
	answer := decodeTransferAnswer(status, raw)
	w.lastTransferAnswer = answer
	w.lastRefusal, w.lastStatus = answer.Refusal, answer.Status
	return nil
}

func (w *World) QueryTransfer(ctx context.Context, as Caller, transferID string) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	status, raw, err := w.rawCall(ctx, as, http.MethodGet, "/transfers/"+url.PathEscape(transferID), nil)
	if err != nil {
		return err
	}
	answer := decodeTransferAnswer(status, raw)
	if answer.TransferID == "" {
		answer.TransferID = transferID
	}
	w.lastTransferAnswer = answer
	w.lastRefusal, w.lastStatus = answer.Refusal, answer.Status
	return nil
}

// PollTransferUntilTerminal is the walking skeleton's own polling loop --
// still a real HTTP driving-port call each iteration, per the sync/async
// contract (brief.md § Sync vs. async settlement). Bounded by the design's
// own documented worst-case time-to-terminal (~204s); this suite polls far
// more often and gives up sooner, since a RED scaffold answers instantly
// (501) rather than genuinely retrying -- once DELIVER implements
// SendTransfer/attemptLeg for real, this loop is what proves settlement
// without the acceptance suite itself sleeping on production timing.
func (w *World) PollTransferUntilTerminal(ctx context.Context, as Caller, transferID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := w.QueryTransfer(ctx, as, transferID); err != nil {
			return err
		}
		switch w.lastTransferAnswer.TxStatus {
		case StatusSettled, StatusReversed, StatusReversalFailed:
			return nil
		}
		if w.lastTransferAnswer.Refusal != "" {
			// A scaffold (or a genuine refusal) never reaches a terminal
			// status -- stop polling and let the Then assertion report the
			// real reason, rather than spinning until timeout.
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("transfer %q did not reach a terminal state within %s (last status=%q refusal=%q)",
				transferID, timeout, w.lastTransferAnswer.TxStatus, w.lastTransferAnswer.Refusal)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// PollTransferUntilStatusLeaves polls (short, bounded) until the transfer's
// reported status is no longer `from` -- unlike PollTransferUntilTerminal,
// this does NOT wait for a terminal state (settled/reversed); it is for
// observing an INTERMEDIATE transition (e.g. pending -> retrying) right
// after a fault-injection Given step races the inline goroutine spawned by
// SendCrossTenantTransfer (spawnForwardLegs, internal/app/
// transfer_coordinator.go). That race is the same one documented on
// InjectLegFault/SimulateCrashBeforeForwardLegAttempt: the fault/crash
// registration is a separate, later HTTP round-trip against the already-
// spawned goroutine, so a bare, non-polling QueryTransfer immediately after
// registration is only safe as long as the goroutine's own inline attempt
// hasn't landed yet -- which depends entirely on inlineAttemptGraceWindow,
// a production-side constant this test file does not own. A short poll here
// tolerates that window widening (companion production-side fix) without
// coupling this test's pass/fail to a race margin. Costs nothing when the
// transition has already happened by the first poll (single QueryTransfer,
// no sleep).
func (w *World) PollTransferUntilStatusLeaves(ctx context.Context, as Caller, transferID string, from TransferStatus, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := w.QueryTransfer(ctx, as, transferID); err != nil {
			return err
		}
		if w.lastTransferAnswer.TxStatus != from {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("transfer %q did not leave status %q within %s (last status=%q refusal=%q)",
				transferID, from, timeout, w.lastTransferAnswer.TxStatus, w.lastTransferAnswer.Refusal)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// --- driving-port calls: intra-tenant account setup (existing, unmodified) -

// OpenAccount opens an account through the existing, unmodified POST
// /accounts port -- this feature introduces no change to it (every leg is
// still, structurally, a plain intra-tenant Post call).
func (w *World) OpenAccount(ctx context.Context, tenant TenantName, name AccountName, kind AccountKind) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	status, raw, err := w.rawCall(ctx, AsTenant(tenant), http.MethodPost, "/accounts",
		map[string]any{"account_id": string(name), "type": string(kind)})
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("opening account %q for tenant %q as a Given-side precondition failed: status=%d body=%s", name, tenant, status, raw)
	}
	return nil
}

// FundAccount moves value from one of a tenant's own accounts to another --
// the existing, unmodified intra-tenant POST /transfers port.
func (w *World) FundAccount(ctx context.Context, tenant TenantName, to AccountName, amount Money, from AccountName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	status, raw, err := w.rawCallWithKey(ctx, AsTenant(tenant), http.MethodPost, "/transfers",
		map[string]any{"from": string(from), "to": string(to), "amount": amount.String()},
		"fund-"+string(tenant)+"-"+string(from)+"-"+string(to)+"-"+fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("funding account %q for tenant %q as a Given-side precondition failed: status=%d body=%s", to, tenant, status, raw)
	}
	return nil
}

// AssertAccountBalance re-reads an account's balance as its owning tenant,
// through the existing, unmodified GET /accounts/{id} port.
func (w *World) AssertAccountBalance(ctx context.Context, tenant TenantName, account AccountName, want Money) error {
	_, raw, err := w.rawCall(ctx, AsTenant(tenant), http.MethodGet, "/accounts/"+url.PathEscape(string(account)), nil)
	if err != nil {
		return err
	}
	var payload struct {
		Balance string `json:"balance"`
	}
	_ = json.Unmarshal(raw, &payload)
	got := ParseMoney(payload.Balance)
	if payload.Balance == "" {
		got = 0
	}
	if got != want {
		return fmt.Errorf("expected tenant %q's account %q balance to read %s, got %s (raw: %s)", tenant, account, want, got, raw)
	}
	return nil
}

// --- composite Given-side scenario builders ---------------------------------

// seedSettlingTransfer builds the full precondition chain a slice 03/04/05
// scenario needs -- provision both tenants, authorize the pair, fund the
// sender, register an alias, and send one transfer -- so each scenario's own
// Given line stays a single, readable sentence (Pillar 2: chained narrative,
// reusing the same driving-port calls slice 01/02's own Given steps use,
// never a bespoke fixture shortcut). The transfer itself is a real HTTP call
// against a RED-scaffolded route (§ package doc); this builder does not
// paper over that -- a scenario built on it fails at its own Then assertion
// for the correct, stated reason.
func (w *World) seedSettlingTransfer(ctx context.Context, from, to TenantName) error {
	return w.seedSettlingTransferWithKey(ctx, from, to, IdempotencyKey(fmt.Sprintf("seed-%d", time.Now().UnixNano())))
}

// seedSettlingTransferWithKey is seedSettlingTransfer's own keyed variant --
// added 2026-09-08 (milestone-04 back-propagation fix, gap 5) for the one
// scenario ("A resend of the same idempotency key after reversal is treated
// as the original request") whose own When step needs the ORIGINAL send to
// have used a SPECIFIC, Gherkin-named idempotency key rather than the
// internally-generated "seed-<timestamp>" one, so the later resend can match
// it exactly. seedSettlingTransfer itself is now a thin wrapper over this.
func (w *World) seedSettlingTransferWithKey(ctx context.Context, from, to TenantName, key IdempotencyKey) error {
	w.lastTransferFrom, w.lastTransferTo = from, to
	if _, ok := w.tenants[from]; !ok {
		if err := w.GivenTenantProvisioned(ctx, from); err != nil {
			return err
		}
	}
	if _, ok := w.tenants[to]; !ok {
		if err := w.GivenTenantProvisioned(ctx, to); err != nil {
			return err
		}
	}
	if err := w.GivenPairAuthorized(ctx, from, to); err != nil {
		return err
	}
	treasury := AccountName(string(from) + "-treasury")
	senderWallet := AccountName(string(from) + "-wallet")
	receiverWallet := AccountName(string(to) + "-wallet-ops")
	alias := AliasName(string(to) + "-payout")

	if _, _, err := w.rawCall(ctx, AsTenant(from), http.MethodPost, "/accounts",
		map[string]any{"account_id": string(treasury), "type": "system"}); err != nil {
		return err
	}
	if _, _, err := w.rawCall(ctx, AsTenant(from), http.MethodPost, "/accounts",
		map[string]any{"account_id": string(senderWallet), "type": "wallet"}); err != nil {
		return err
	}
	if _, _, err := w.rawCallWithKey(ctx, AsTenant(from), http.MethodPost, "/transfers",
		map[string]any{"from": string(treasury), "to": string(senderWallet), "amount": "500.00"},
		"seed-fund-"+string(from)); err != nil {
		return err
	}
	if _, _, err := w.rawCall(ctx, AsTenant(to), http.MethodPost, "/accounts",
		map[string]any{"account_id": string(receiverWallet), "type": "wallet"}); err != nil {
		return err
	}
	if err := w.RegisterAlias(ctx, from, alias, to, receiverWallet); err != nil {
		return err
	}
	return w.SendCrossTenantTransfer(ctx, from, ParseMoney("50.00"), alias, key)
}

// exhaustRetryPollTimeout bounds exhaustLeg2AndReverse's real-time poll.
// backoffForAttempt's own schedule (transfer_coordinator.go: 1s/2s/4s/8s,
// backoffJitterSpread=0.2) sums to a 15s base across leg 2's 4 scheduled
// retries (retryBudget=5 attempts total), up to 18s worst case with jitter.
// The self-rescheduling goroutine (scheduleRetry) drives every one of those
// retries on its own real time.Sleep -- there is no faster way to observe
// the sequence land than waiting for it in real wall-clock time. 25s gives
// ~7s of margin over the 18s jittered worst case for attempt/network
// overhead (each attempt's own HTTP round-trip to the ledger, well under
// attemptTimeout=10s in practice).
const exhaustRetryPollTimeout = 25 * time.Second

// exhaustLeg2AndReverse arms leg 2 to fail its FULL retry budget (mirrors
// gap 1's own InjectLegFaultCount(..., 2, 5) fix) then waits, in real time,
// for the transfer to reach a terminal status. Updated 2026-09-08
// (milestone-04 back-propagation fix, gaps 3/4/5 -- corrected from an
// earlier interrupted dispatch's tick-loop approach): attemptLeg's failure
// path (handleLegFailure -> scheduleRetry, transfer_coordinator.go) already
// self-reschedules each retry via a real time.Sleep in its own detached
// goroutine once the fault is armed below -- the retry ticker
// (RunRetryTickerOnce/processDueTransfers) exists ONLY to drive progress
// after a simulated crash, where that goroutine never got the chance to
// run. Calling the ticker back-to-back here would be a near-total no-op:
// the row is not independently "due" between the goroutine's own scheduled
// attempts, since next_attempt_at reflects the SAME backoff schedule the
// goroutine is already honoring. A real-time bounded poll is the only
// mechanism that actually observes this self-driven sequence land.
func (w *World) exhaustLeg2AndReverse(ctx context.Context) error {
	transferID := w.LastTransferAnswer().TransferID
	if err := w.InjectLegFaultCount(ctx, transferID, 2, 5); err != nil {
		return err
	}
	return w.PollTransferUntilTerminal(ctx, PlatformAdmin(), transferID, exhaustRetryPollTimeout)
}

// driveTransferToReversed composes seedSettlingTransfer with
// exhaustLeg2AndReverse -- the shared implementation behind every
// milestone-04 Given that needs a GENUINELY reversed transfer (gaps 3 and 4).
func (w *World) driveTransferToReversed(ctx context.Context, from, to TenantName) error {
	if err := w.seedSettlingTransfer(ctx, from, to); err != nil {
		return err
	}
	return w.exhaustLeg2AndReverse(ctx)
}

// driveTransferToReversedWithKey is driveTransferToReversed's own keyed
// variant (gap 5), seeding under a Gherkin-supplied idempotency key instead
// of an internally-generated one.
func (w *World) driveTransferToReversedWithKey(ctx context.Context, from, to TenantName, key IdempotencyKey) error {
	if err := w.seedSettlingTransferWithKey(ctx, from, to, key); err != nil {
		return err
	}
	return w.exhaustLeg2AndReverse(ctx)
}

// resolveTransferID lets a Then/When step refer to a transfer by the story's
// own illustrative id ("xfr_1", as written in the UAT scenarios) while this
// suite only ever knows the real, server-generated id -- resolved to the
// last transfer this World produced. "a nonexistent id" and "a nonexistent
// transfer_id" are the one phrasing that deliberately does NOT resolve, so
// the adversarial scenarios can assert against a genuinely unissued id.
func (w *World) resolveTransferID(text string) string {
	switch text {
	case "a nonexistent id", "a nonexistent transfer_id":
		return "xfr_never_issued"
	default:
		if w.lastTransferAnswer.TransferID != "" {
			return w.lastTransferAnswer.TransferID
		}
		return text
	}
}

// parseAdversarialCaller coerces slice 05's own scenario-outline phrasing
// ("third-party tenant") into a Caller -- the one unauthorized tenant every
// isolation scenario in this suite provisions under a fixed name.
func (w *World) parseAdversarialCaller(text string) Caller {
	switch text {
	case "third-party tenant":
		return AsTenant(TenantName("tnt_carter"))
	default:
		return AsTenant(TenantName(text))
	}
}

// --- driving-port calls: trial balance (I3) ---------------------------------

// AssertTrialBalanceHolds asks the platform-wide "do the books balance"
// question through the existing, unmodified GET /health/trial-balance port
// -- the same driving port multitenancy's own CheckTrialBalance uses, called
// unscoped here since a reversal's own I3 obligation spans every account it
// touched (sender's tenant, receiver's tenant, and the platform's own
// reserved ledger), not one tenant's slice alone. Combines the call and the
// assertion into one composition-root method (Mandate-12 criterion 3 -- a
// step body delegates, it never inlines the comparison itself).
func (w *World) AssertTrialBalanceHolds(ctx context.Context) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	status, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodGet, "/health/trial-balance", nil)
	if err != nil {
		return err
	}
	var payload struct {
		Verdict string `json:"verdict"`
	}
	_ = json.Unmarshal(raw, &payload)
	if Verdict(payload.Verdict) != BooksBalanceYes {
		return fmt.Errorf("expected the trial balance to hold after the reversal (I3), got verdict=%q status=%d (raw: %s)",
			payload.Verdict, status, raw)
	}
	return nil
}

// --- fault-injection seams: wired as of step 03-01 (see package doc) -------

// InjectLegFault forces the named transfer's next attempt at the named leg
// to fail with a simulated transient fault, over the test-only HTTP seam
// (internal/adapters/http/testonly_faults.go).
func (w *World) InjectLegFault(ctx context.Context, transferID string, leg int) error {
	return w.callTestOnlyFaultSeam(ctx, "/testonly/faults/leg", map[string]any{
		"transfer_id": transferID,
		"leg":         leg,
	})
}

// InjectLegFaultCount forces the named transfer's next `count` consecutive
// attempts at the named leg to each fail with a simulated transient fault,
// over the same test-only HTTP seam as InjectLegFault (which is the count=1
// case) -- added 2026-09-08 for "Exhausting attempt 1 and 2 before
// succeeding on attempt 3", whose own Gherkin needs a SPECIFIC, bounded
// number of consecutive failures rather than "the next attempt", which
// InjectLegFault's one-shot semantics could not express.
func (w *World) InjectLegFaultCount(ctx context.Context, transferID string, leg, count int) error {
	return w.callTestOnlyFaultSeam(ctx, "/testonly/faults/leg", map[string]any{
		"transfer_id": transferID,
		"leg":         leg,
		"fail_count":  count,
	})
}

// SimulateCrashBeforeForwardLegAttempt simulates a process crash in the
// FORWARD-path window: after a leg has committed but before the next leg's
// first attempt has ever run. See package doc above.
func (w *World) SimulateCrashBeforeForwardLegAttempt(ctx context.Context, transferID string) error {
	return w.callTestOnlyFaultSeam(ctx, "/testonly/faults/crash-forward", map[string]any{
		"transfer_id": transferID,
	})
}

// SimulateCrashBeforeReversalAttempt simulates a process crash in the
// REVERSAL-path window: after a later leg's compensating reversal has
// committed but before an earlier leg's reversal has ever been attempted.
// Semantically distinct from SimulateCrashBeforeForwardLegAttempt -- see
// package doc above.
func (w *World) SimulateCrashBeforeReversalAttempt(ctx context.Context, transferID string) error {
	return w.callTestOnlyFaultSeam(ctx, "/testonly/faults/crash-reversal", map[string]any{
		"transfer_id": transferID,
	})
}

// RunRetryTickerOnce single-steps processDueTransfers exactly once -- no
// real wall-clock sleep needed to observe one tick's effect.
//
// Refreshes lastTransferAnswer with a single fresh QueryTransfer right after
// the tick call returns -- NOT a poll. processDueTransfers (internal/app/
// transfer_coordinator.go) is fully synchronous per call: it loops directly
// over attemptDueLegs/attemptLeg with no goroutine spawn, and the
// /testonly/tick handler (internal/adapters/http/testonly_faults.go) calls
// ProcessDueTransfersOnce synchronously before writing its HTTP response. So
// by the time this call returns, every leg the ticker was going to attempt
// this tick has already been attempted -- a single re-query is enough to
// observe the outcome; a bounded poll would only mask a genuine synchrony
// regression instead of catching it.
func (w *World) RunRetryTickerOnce(ctx context.Context, as Caller, transferID string) error {
	if err := w.callTestOnlyFaultSeam(ctx, "/testonly/tick", nil); err != nil {
		return err
	}
	return w.QueryTransfer(ctx, as, transferID)
}

// SeedTransfersDueForRetry seeds N synthetic, already-due transfer_state
// rows directly through the store -- the batch-limit scenario's own need.
func (w *World) SeedTransfersDueForRetry(ctx context.Context, count int) error {
	return w.callTestOnlyFaultSeam(ctx, "/testonly/seed-due", map[string]any{
		"count": count,
	})
}

// callTestOnlyFaultSeam is every fault-injection method's own shared
// transport: a real HTTP call to the test-only seam, as the platform admin
// (the same credential every other cross-cutting route in this router
// requires) -- mirroring this suite's own "every driving-port call goes
// over the real HTTP server" rule (package doc above), never a direct Go
// reference into the running process.
func (w *World) callTestOnlyFaultSeam(ctx context.Context, path string, body any) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	status, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("test-only fault seam call to %q failed: status=%d body=%s", path, status, raw)
	}
	return nil
}

// --- platform/settlement account naming (test-infra duplication) -----------
//
// platformMirrorAccountIDConvention and settlementAccountID duplicate two
// narrow production naming conventions (internal/app/transfer_coordinator.go's
// own platformMirrorAccountID and settlementAccountName), not business logic
// -- same precedent as this file's own package-doc header re: startPostgres/
// serve/StartAgainstEmptyStore. World never holds a Go reference into the
// running coordinator (package doc, "everything through HTTP"), so it cannot
// call the unexported production functions directly and instead mirrors the
// one string format each produces.
const settlementAccountID = "settlement"

func platformMirrorAccountIDConvention(businessTenantID string) string {
	return "platform-" + businessTenantID
}

// entryWireView is the wire shape GET /accounts/{id}/entries answers with,
// trimmed to the fields the milestone-03 back-propagation fix's own Then
// steps need (handlers.go's entriesToWire).
type entryWireView struct {
	TransactionID string `json:"transaction_id"`
	Counterparty  string `json:"counterparty"`
	Amount        string `json:"amount"`
}

// InspectPlatformMirrorEntries queries the sender's own tnt_platform mirror
// account's entry log -- the "platform account" the milestone-03 Gherkin
// names in "Retries never produce a duplicate posted leg" -- and caches the
// answer for the following Then step to count against. A real HTTP round
// trip, not a status-shaped proxy (Mandate 8).
func (w *World) InspectPlatformMirrorEntries(ctx context.Context) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	from, ok := w.tenants[w.lastTransferFrom]
	if !ok {
		return fmt.Errorf("InspectPlatformMirrorEntries: sender tenant %q not resolved", w.lastTransferFrom)
	}
	account := platformMirrorAccountIDConvention(from.ID)
	_, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodGet, "/accounts/"+url.PathEscape(account)+"/entries", nil)
	if err != nil {
		return err
	}
	var payload struct {
		Entries []entryWireView `json:"entries"`
	}
	_ = json.Unmarshal(raw, &payload)
	w.inspectedEntries = payload.Entries
	return nil
}

// AssertExactlyOnePostedLegEntry counts, from the last inspected entry log,
// how many entries the sender's platform mirror account posted to the
// receiver's own platform mirror -- leg 2's own movement -- and asserts
// exactly one, the real proof that a retried leg never produces a second
// posting (brief.md's dual-idempotency mechanism, per-leg IdempotencyStore
// replay), replacing the earlier vacuous AssertLegStatus(2, LegPosted) check.
func (w *World) AssertExactlyOnePostedLegEntry(leg int) error {
	to, ok := w.tenants[w.lastTransferTo]
	if !ok {
		return fmt.Errorf("AssertExactlyOnePostedLegEntry: receiver tenant %q not resolved", w.lastTransferTo)
	}
	counterparty := platformMirrorAccountIDConvention(to.ID)
	matched := 0
	for _, entry := range w.inspectedEntries {
		if entry.Counterparty == counterparty {
			matched++
		}
	}
	if matched != 1 {
		return fmt.Errorf("expected exactly 1 posted transaction for leg %d (platform mirror -> %q), got %d across %d inspected entries",
			leg, counterparty, matched, len(w.inspectedEntries))
	}
	return nil
}

// InspectSenderSettlementBalance queries the sender's own settlement
// account -- the account Leg 1 posts into, and what this suite's own
// Gherkin calls "the platform account" from the sending tenant's point of
// view -- and caches its balance for the following Then step ("Funds stay
// parked, not lost, while a leg is retrying").
func (w *World) InspectSenderSettlementBalance(ctx context.Context) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	_, raw, err := w.rawCall(ctx, AsTenant(w.lastTransferFrom), http.MethodGet, "/accounts/"+settlementAccountID, nil)
	if err != nil {
		return err
	}
	var payload struct {
		Balance string `json:"balance"`
	}
	_ = json.Unmarshal(raw, &payload)
	got := ParseMoney(payload.Balance)
	if payload.Balance == "" {
		got = 0
	}
	w.inspectedBalance = got
	return nil
}

// AssertInspectedBalanceReflects compares the last InspectSenderSettlementBalance
// answer against the amount the scenario's own Then step names.
func (w *World) AssertInspectedBalanceReflects(want Money) error {
	if w.inspectedBalance != want {
		return fmt.Errorf("expected the platform account's balance to reflect exactly %s received via leg 1, pending onward movement, got %s",
			want, w.inspectedBalance)
	}
	return nil
}

func (w *World) CorruptStoredBalance(ctx context.Context, account AccountName, by Money) error {
	return postgres.AttemptOutOfBandChange(ctx, w.privilegedDSN, "alter_stored_balance", map[string]any{
		"account_id": string(account),
		"delta":      int64(by),
	})
}

// --- transport ---------------------------------------------------------

func (w *World) rawCall(ctx context.Context, as Caller, method, path string, body any) (int, []byte, error) {
	return w.rawCallWithKey(ctx, as, method, path, body, "")
}

func (w *World) rawCallWithKey(ctx context.Context, as Caller, method, path string, body any, key string) (int, []byte, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, w.server.URL+path, bytes.NewReader(encoded))
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	w.authenticate(request, as)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := w.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode, raw, nil
}

func (w *World) authenticate(request *http.Request, as Caller) {
	switch as.Role {
	case NoCredRole:
		return
	case UnissuedRole:
		request.Header.Set("Authorization", "Bearer never-issued-key")
	case AdminRole:
		request.Header.Set("Authorization", "Bearer "+w.adminKey)
	case TenantRole:
		// A not-yet-resolved tenant (e.g. a scenario naming a tenant that
		// was never provisioned) presents a synthetic, certainly-unissued
		// bearer -- mirrors multitenancy's own precedent, so a RED scaffold
		// fails at the Then assertion rather than aborting the scenario.
		if record, ok := w.tenants[as.Tenant]; ok {
			request.Header.Set("Authorization", "Bearer "+record.Key)
			return
		}
		request.Header.Set("Authorization", "Bearer unresolved-tenant-"+string(as.Tenant))
	}
}

// --- decoding ------------------------------------------------------------

func decodeLinkAnswer(status int, raw []byte) LinkAnswer {
	answer := LinkAnswer{Status: status, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	if status >= 200 && status < 300 {
		answer.LinkID, _ = payload["link_id"].(string)
		if s, ok := payload["status"].(string); ok {
			answer.Status_ = ParseLinkStatus(s)
		}
		return answer
	}
	if kind, ok := payload["error"].(string); ok {
		answer.Refusal = RefusalKind(kind)
	}
	return answer
}

func decodeAliasAnswer(status int, raw []byte) AliasAnswer {
	answer := AliasAnswer{Status: status, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	if kind, ok := payload["error"].(string); ok {
		answer.Refusal = RefusalKind(kind)
	}
	return answer
}

func decodeTransferAnswer(status int, raw []byte) TransferAnswer {
	answer := TransferAnswer{Status: status, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	answer.TransferID, _ = payload["transfer_id"].(string)
	if s, ok := payload["status"].(string); ok {
		answer.TxStatus = ParseTransferStatus(s)
	}
	answer.Reason, _ = payload["reason"].(string)
	if kind, ok := payload["error"].(string); ok {
		answer.Refusal = RefusalKind(kind)
	}
	for i, key := range []string{"leg1", "leg2", "leg3"} {
		if leg, ok := payload[key].(map[string]any); ok {
			status, _ := leg["status"].(string)
			answer.Legs = append(answer.Legs, LegView{Leg: i + 1, Status: LegStatus(status)})
		}
	}
	return answer
}

// --- Then assertions -----------------------------------------------------
//
// None routed through statedelta.AssertStateDelta -- this suite drives
// scenarios from TestMain via godog, not from a *testing.T subtest,
// identical precedent to multitenancy/world.go's own "Then assertions"
// section. Universe-bound discipline (Mandate 8) is honoured directly:
// every assertion compares one declared, port-exposed observable against
// its expected value.

func (w *World) AssertLinkCreated(a, b TenantName, want LinkStatus) error {
	link := w.links[canonicalPair(a, b)]
	if !link.Created() {
		return fmt.Errorf("expected a link to be created between %q and %q, got status=%d refusal=%q (raw: %s)",
			a, b, link.Status, link.Refusal, link.Raw)
	}
	if link.Status_ != want {
		return fmt.Errorf("expected link between %q and %q to read %q, got %q", a, b, want, link.Status_)
	}
	return nil
}

func (w *World) AssertLinkReads(a, b TenantName, want LinkStatus) error {
	link := w.links[canonicalPair(a, b)]
	if link.Status_ != want {
		return fmt.Errorf("expected link between %q and %q to read %q, got %q (raw: %s)", a, b, want, link.Status_, link.Raw)
	}
	return nil
}

func (w *World) AssertLinkRefused(kind RefusalKind) error {
	if w.lastLinkAnswer.Refusal != kind {
		return fmt.Errorf("expected the tenant-link response to be refused as %q, got refusal=%q status=%d (raw: %s)",
			kind, w.lastLinkAnswer.Refusal, w.lastLinkAnswer.Status, w.lastLinkAnswer.Raw)
	}
	return nil
}

func (w *World) AssertNoLinkCreated(a, b TenantName) error {
	link, known := w.links[canonicalPair(a, b)]
	if known && link.Created() && link.LinkID != w.priorLinkAnswer.LinkID {
		return fmt.Errorf("expected no new link between %q and %q, but one was created (id=%q)", a, b, link.LinkID)
	}
	return nil
}

func (w *World) AssertAliasRegistered(alias AliasName) error {
	if !w.lastAliasAnswer.Registered() {
		return fmt.Errorf("expected alias %q to register, got status=%d refusal=%q (raw: %s)",
			alias, w.lastAliasAnswer.Status, w.lastAliasAnswer.Refusal, w.lastAliasAnswer.Raw)
	}
	return nil
}

func (w *World) AssertAliasRefused(kind RefusalKind) error {
	if w.lastAliasAnswer.Refusal != kind {
		return fmt.Errorf("expected the alias response to be refused as %q, got refusal=%q status=%d (raw: %s)",
			kind, w.lastAliasAnswer.Refusal, w.lastAliasAnswer.Status, w.lastAliasAnswer.Raw)
	}
	return nil
}

func (w *World) AssertTransferRefused(kind RefusalKind) error {
	if w.lastRefusal != kind {
		return fmt.Errorf("expected the response to be refused as %q, got refusal=%q status=%d (raw: %s)",
			kind, w.lastRefusal, w.lastStatus, w.lastTransferAnswer.Raw)
	}
	return nil
}

func (w *World) AssertTransferStatus(want TransferStatus) error {
	if w.lastTransferAnswer.TxStatus != want {
		return fmt.Errorf("expected transfer status %q, got %q (raw: %s)", want, w.lastTransferAnswer.TxStatus, w.lastTransferAnswer.Raw)
	}
	return nil
}

func (w *World) AssertLegStatus(leg int, want LegStatus) error {
	got := w.lastTransferAnswer.LegStatusOf(leg)
	if got != want {
		return fmt.Errorf("expected leg %d status %q, got %q (raw: %s)", leg, want, got, w.lastTransferAnswer.Raw)
	}
	return nil
}

func (w *World) AssertReason(want string) error {
	if w.lastTransferAnswer.Reason != want {
		return fmt.Errorf("expected reason %q, got %q (raw: %s)", want, w.lastTransferAnswer.Reason, w.lastTransferAnswer.Raw)
	}
	return nil
}

func (w *World) LastTransferAnswer() TransferAnswer { return w.lastTransferAnswer }
func (w *World) LastLinkAnswer() LinkAnswer         { return w.lastLinkAnswer }
func (w *World) LastRefusal() RefusalKind           { return w.lastRefusal }
