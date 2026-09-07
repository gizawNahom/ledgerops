package multitenancy

// World is the acceptance suite's composition root (Tier A, Mandate 10). It
// wires the PRODUCTION composition root: the real chi router
// (internal/adapters/http), the real requireOperatorKey middleware, the real
// postgres adapter against a real PostgreSQL 16 from Testcontainers (WS
// strategy C, per feature-delta.md § Wave: DISCUSS / WS strategy). No fake
// stands in for TenantRepository or the credential mechanism — isolation is a
// property of the real store's constraint and query behavior, and a fake
// would model the very thing under test (same rationale ledger-core's own
// composition root uses).
//
// The clock and id generator are NOT faked here (unlike ledgercore's Ledger):
// no scenario in this feature's scope needs a pinned timestamp or a
// predictable generated id — every assertion is about WHICH tenant can see
// WHAT, not about a specific id/timestamp value.
//
// State machine this suite exercises (C2 of the AT completeness taxonomy):
//
//	TENANT      unprovisioned --provision--> provisioned
//	            illegal from unprovisioned: open an account under it, check its
//	            trial balance                                          (refused)
//	            illegal from provisioned: provision again under the same name
//	                                       (refused, tenant_already_exists)
//
//	ACCOUNT     scoped to (tenant, account_id) — reuses ledgercore's own
//	            unopened --open--> open state machine, one instance per tenant,
//	            with a second axis: open in tenant A is invisible from tenant B
//	            (account_not_found, never a distinguishable forbidden — ADR-009)
//
// Every transition above has at least one scenario; every state has at least
// one illegal-event scenario.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
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
	"ledgerops/tests/common/statedelta"
)

type tenantRecord struct {
	ID  string
	Key string
}

// World is the composition root. See package-level doc above.
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

	adminKey string // the existing platform-admin credential (OperatorKey, D8)

	tenants           map[TenantName]tenantRecord
	lastAccountTenant map[AccountName]TenantName
	actingAs          Caller

	lastTenantAnswer  TenantAnswer
	priorTenantAnswer TenantAnswer

	lastAnswer  Answer
	priorAnswer Answer

	lastBooks  BooksReport
	priorBooks BooksReport

	// lastRefusal is the SSOT every "Then the response is refused as X" step
	// reads from, regardless of which driving port produced the refusal
	// (provisioning, account, transfer, or verdict). Set to "" on any
	// accepted call.
	lastRefusal RefusalKind

	// lastStatus mirrors lastRefusal: the HTTP status of whichever call most
	// recently ran, so AssertRefused's failure message is accurate regardless
	// of which driving port (tenant/account/books) produced it.
	lastStatus int

	// openAnswers accumulates every OpenAccount result in a scenario, so a
	// "both accounts are opened successfully" Then can check the last two
	// without the second call's Answer overwriting the first's.
	openAnswers []Answer
}

// Answer is what the account/transfer driving ports hand back — the
// observable slice 02's scenarios assert on. Deliberately a small subset of
// ledgercore's own Answer shape: this feature's scenarios never assert on
// legs/transaction ids, only on which tenant can see what.
type Answer struct {
	Status       int
	Accepted     bool
	AccountID    string
	Balance      Money
	Refusal      RefusalKind
	NamedAccount string
	Raw          string
}

// NewWorld builds the composition root.
func NewWorld() *World {
	return &World{
		adminKey: "test-operator-key",
		actingAs: PlatformAdmin(),
		tenants:  map[TenantName]tenantRecord{},
	}
}

// --- lifecycle ---------------------------------------------------------

// StartAgainstEmptyStore brings up a real PostgreSQL 16 container, migrates
// the schema from zero as the privileged role, and serves the production
// router over a real socket as the application role — identical shape to
// ledgercore's own StartAgainstEmptyStore, duplicated rather than imported
// because startPostgres is unexported in that package (test-infra bootstrap
// duplication, not a Mandate-12 business-logic violation — see domain_types.go
// header for where this project draws that line).
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
	})
	w.server = httptest.NewServer(handler)
	w.client = w.server.Client()
	return nil
}

// EnsureStarted brings the store and server up if a scenario's first Given
// did not already do so explicitly (e.g. "tenant X has been provisioned" as
// the very first step, with no separate "a fresh store" Given).
func (w *World) EnsureStarted(ctx context.Context) error {
	if w.server != nil {
		return nil
	}
	return w.StartAgainstEmptyStore(ctx)
}

// GivenTenantProvisioned is the Given-side convenience every scenario's
// second-and-later "tenant X has been provisioned" step uses: ensure the
// store is up, act as the platform admin, provision, and leave actingAs at
// the admin credential (a following When step names its own caller
// explicitly, per Mandate 2 — Given sets up preconditions, never the
// expected output).
func (w *World) GivenTenantProvisioned(ctx context.Context, name TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.ActAs(PlatformAdmin())
	return w.ProvisionTenant(ctx, name)
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

// --- containers ----------------------------------------------------------
//
// One PostgreSQL 16 instance per test binary, not per scenario. Booting the
// container and migrating from zero costs ~2.5s and now happens once; each
// scenario then gets its own database cloned off the migrated template,
// which is a file copy inside the already-running instance and costs a small
// fraction of that.
//
// Isolation is unchanged, which is the point: a freshly cloned database
// inherits nothing from a prior scenario, so the two-tenant environment
// still starts from a known-empty store. What a shared *database* would have
// broken — one scenario passing on rows or drift another left behind — a
// per-scenario database does not.
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

// uuidLike is not a fake port — no clock/id port is faked in this suite (see
// package doc). It is only how the suite fabricates transaction ids the
// production IDGenerator would otherwise supply via crypto/rand; no scenario
// in this feature's scope asserts on a specific value.
func uuidLike() string {
	uuidMu.Lock()
	defer uuidMu.Unlock()
	uuidCounter++
	return fmt.Sprintf("mt_%d_%d", time.Now().UnixNano(), uuidCounter)
}

// --- the observable universe ----------------------------------------------

// Universe is the set of port-exposed observable names this suite reasons
// about, for the tenants and accounts a scenario names. Every name is
// readable through a driving port — none names an internal field (nw-distill
// § Example, good vs bad Universe).
func Universe(tenants []TenantName, accounts ...AccountName) []string {
	names := []string{}
	for _, t := range tenants {
		names = append(names, fmt.Sprintf("tenant.%s.provisioned", t))
	}
	for _, a := range accounts {
		names = append(names, fmt.Sprintf("account.%s.balance", a))
	}
	sort.Strings(names)
	return names
}

// CaptureUniverse reads every observable in the universe through the driving
// ports.
func (w *World) CaptureUniverse(ctx context.Context, tenants []TenantName, accounts ...AccountName) (statedelta.Snapshot, error) {
	snapshot := statedelta.Snapshot{}
	for _, t := range tenants {
		_, provisioned := w.tenants[t]
		snapshot[fmt.Sprintf("tenant.%s.provisioned", t)] = provisioned
	}
	for _, a := range accounts {
		balance, err := w.readBalance(ctx, w.tenantOf(a), a)
		if err != nil {
			return nil, err
		}
		snapshot[fmt.Sprintf("account.%s.balance", a)] = balance
	}
	return snapshot, nil
}

// tenantOf is a scenario-authoring convenience: within one scenario an
// account name is opened by exactly one tenant, so the last tenant seen to
// open it is who CaptureUniverse reads it back as. Cross-tenant same-name
// collisions (I9/I10 scenarios) never call CaptureUniverse on the shared
// name — they assert on the Answer directly.
func (w *World) tenantOf(account AccountName) TenantName {
	return w.lastAccountTenant[account]
}

// --- driving-port calls: provisioning (slice 01) ---------------------------

// ProvisionTenant provisions a tenant through the real front door, as
// whichever caller is currently acting.
func (w *World) ProvisionTenant(ctx context.Context, name TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorTenantAnswer = w.lastTenantAnswer
	body := map[string]any{"name": string(name)}
	answer, err := w.callTenants(ctx, http.MethodPost, "/tenants", body)
	w.lastTenantAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	if answer.Provisioned() {
		w.tenants[name] = tenantRecord{ID: answer.TenantID, Key: answer.TenantKey}
	}
	return err
}

// --- driving-port calls: operate within a tenant (slice 02) ----------------

func (w *World) OpenAccount(ctx context.Context, tenant TenantName, name AccountName, kind AccountKind) error {
	return w.OpenAccountAs(ctx, AsTenant(tenant), name, kind)
}

// OpenAccountAs opens an account under an arbitrary caller identity — used
// both by OpenAccount (a legitimate tenant) and by the malformed/missing
// credential scenarios, which never resolve to a specific tenant.
func (w *World) OpenAccountAs(ctx context.Context, as Caller, name AccountName, kind AccountKind) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	if as.role == tenantRole {
		w.rememberAccountTenant(name, as.tenant)
	}
	w.priorAnswer = w.lastAnswer
	body := map[string]any{"account_id": string(name), "type": string(kind)}
	answer, err := w.callAs(ctx, as, http.MethodPost, "/accounts", body)
	w.lastAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	w.openAnswers = append(w.openAnswers, answer)
	return err
}

func (w *World) FundAccount(ctx context.Context, tenant TenantName, to AccountName, amount Money, from AccountName) error {
	return w.Transfer(ctx, tenant, from, to, amount)
}

func (w *World) Transfer(ctx context.Context, tenant TenantName, from, to AccountName, amount Money) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorAnswer = w.lastAnswer
	body := map[string]any{"from": string(from), "to": string(to), "amount": amount.String()}
	answer, err := w.callAsWithKey(ctx, AsTenant(tenant), http.MethodPost, "/transfers", body, "mt-"+string(tenant)+"-"+string(from)+"-"+string(to))
	w.lastAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	return err
}

// TransferAcrossTenants submits a transfer under one tenant's credential
// naming an account that belongs to a different tenant — the scenario US-2's
// core adversarial case exists to refuse.
func (w *World) TransferAcrossTenants(ctx context.Context, as TenantName, from AccountName, toTenant TenantName, to AccountName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorAnswer = w.lastAnswer
	body := map[string]any{"from": string(from), "to": string(to), "amount": "1.00"}
	answer, err := w.callAsWithKey(ctx, AsTenant(as), http.MethodPost, "/transfers", body, "mt-cross-"+string(from)+"-"+string(to))
	w.lastAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	return err
}

// RequestAccount reads an account as the given caller — the cross-tenant
// visibility probe.
func (w *World) RequestAccount(ctx context.Context, as Caller, name AccountName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorAnswer = w.lastAnswer
	answer, err := w.callAs(ctx, as, http.MethodGet, "/accounts/"+url.PathEscape(string(name)), nil)
	w.lastAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	return err
}

// RequestEntries reads an account's entries — dual-mode per slice 02's own
// scope: tenant-scoped for a tenant_key caller, unscoped for the admin
// credential (console compatibility).
func (w *World) RequestEntries(ctx context.Context, as Caller, name AccountName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorAnswer = w.lastAnswer
	answer, err := w.callAs(ctx, as, http.MethodGet, "/accounts/"+url.PathEscape(string(name))+"/entries", nil)
	w.lastAnswer = answer
	w.lastRefusal = answer.Refusal
	w.lastStatus = answer.Status
	return err
}

// --- driving-port calls: verify one tenant's books (slice 03) --------------

// CheckTrialBalance asks the tenant-scoped or unscoped question, depending on
// whether tenant is the zero value.
func (w *World) CheckTrialBalance(ctx context.Context, tenant TenantName) error {
	return w.readVerdict(ctx, "/health/trial-balance", tenant)
}

func (w *World) CheckConsoleVerdict(ctx context.Context) error {
	return w.readVerdict(ctx, "/console/verdict", "")
}

func (w *World) readVerdict(ctx context.Context, path string, tenant TenantName) error {
	if err := w.EnsureStarted(ctx); err != nil {
		return err
	}
	w.priorBooks = w.lastBooks
	full := path
	tenantScoped := tenant != ""
	tenantID := ""
	if tenantScoped {
		record, known := w.tenants[tenant]
		if known {
			tenantID = record.ID
		}
		// An unknown tenant name still produces a query string — the id is
		// simply the tenant name itself when it was never provisioned, which
		// the server will not recognize either way. This is what lets the
		// "checking an unprovisioned tenant" scenario reach a genuine
		// tenant_not_found refusal instead of a scenario-authoring panic.
		if !known {
			tenantID = string(tenant)
		}
		full = path + "?tenant_id=" + url.QueryEscape(tenantID)
	}
	status, raw, err := w.rawCall(ctx, PlatformAdmin(), http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	report := decodeBooksReport(status, raw, tenantScoped, tenantID)
	w.lastBooks = report
	w.lastRefusal = report.Refusal
	w.lastStatus = report.Status
	return nil
}

// --- what the suite may never do for real ---------------------------------

// CorruptStoredBalance alters one tenant's stored balance as the privileged
// role — the only role that can, mirroring ledgercore's own precedent
// (D7/OPS-10). Used solely to give slice 03's drift scenario a genuine drift
// to detect.
func (w *World) CorruptStoredBalance(ctx context.Context, account AccountName, by Money) error {
	return postgres.AttemptOutOfBandChange(ctx, w.privilegedDSN, "alter_stored_balance", map[string]any{
		"account_id": string(account),
		"delta":      int64(by),
	})
}

// ActAs selects which credential subsequent calls present, when a scenario
// needs to change identity mid-flow without naming a specific tenant.
func (w *World) ActAs(caller Caller) { w.actingAs = caller }

// --- transport --------------------------------------------------------

func (w *World) rememberAccountTenant(account AccountName, tenant TenantName) {
	if w.lastAccountTenant == nil {
		w.lastAccountTenant = map[AccountName]TenantName{}
	}
	w.lastAccountTenant[account] = tenant
}

func (w *World) readBalance(ctx context.Context, tenant TenantName, account AccountName) (Money, error) {
	_, raw, err := w.rawCall(ctx, AsTenant(tenant), http.MethodGet, "/accounts/"+url.PathEscape(string(account)), nil)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Balance string `json:"balance"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, nil // not a balance-bearing response (e.g. a refusal) — 0 is a safe non-match
	}
	if payload.Balance == "" {
		return 0, nil
	}
	return ParseMoney(payload.Balance), nil
}

func (w *World) callTenants(ctx context.Context, method, path string, body any) (TenantAnswer, error) {
	status, raw, err := w.rawCall(ctx, w.actingAs, method, path, body)
	if err != nil {
		return TenantAnswer{}, err
	}
	return decodeTenantAnswer(status, raw), nil
}

func (w *World) callAs(ctx context.Context, as Caller, method, path string, body any) (Answer, error) {
	status, raw, err := w.rawCall(ctx, as, method, path, body)
	if err != nil {
		return Answer{}, err
	}
	return decodeAnswer(status, raw), nil
}

func (w *World) callAsWithKey(ctx context.Context, as Caller, method, path string, body any, key string) (Answer, error) {
	status, raw, err := w.rawCallWithKey(ctx, as, method, path, body, key)
	if err != nil {
		return Answer{}, err
	}
	return decodeAnswer(status, raw), nil
}

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
	switch as.role {
	case noCredRole:
		return
	case unissuedRole:
		request.Header.Set("Authorization", "Bearer never-issued-key")
	case adminRole:
		request.Header.Set("Authorization", "Bearer "+w.adminKey)
	case tenantRole:
		record, ok := w.tenants[as.tenant]
		if !ok {
			// Provisioning is a RED scaffold today (Mandate 7, router.go
			// scaffold("provision_tenant")): no tenant ever receives a real
			// tenant_key until DELIVER implements it, so every scenario that
			// acts as a tenant reaches this branch during RED. A synthetic,
			// certainly-unissued bearer keeps the call a real HTTP round trip
			// that fails at the Then assertion (RED, MISSING_FUNCTIONALITY) —
			// panicking here would abort the whole scenario (and, under
			// godog's concurrent runner, sibling scenarios) instead of
			// failing for the right reason.
			request.Header.Set("Authorization", "Bearer unresolved-tenant-"+string(as.tenant))
			return
		}
		request.Header.Set("Authorization", "Bearer "+record.Key)
	}
}

func decodeTenantAnswer(status int, raw []byte) TenantAnswer {
	answer := TenantAnswer{Status: status, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	if status == http.StatusCreated {
		answer.TenantID, _ = payload["tenant_id"].(string)
		answer.Name, _ = payload["name"].(string)
		answer.TenantKey, _ = payload["tenant_key"].(string)
		return answer
	}
	if kind, ok := payload["error"].(string); ok {
		answer.Refusal = RefusalKind(kind)
	}
	return answer
}

func decodeAnswer(status int, raw []byte) Answer {
	answer := Answer{Status: status, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	switch {
	case status >= 200 && status < 300:
		answer.Accepted = true
		if id, ok := payload["account_id"].(string); ok {
			answer.AccountID = id
		}
		if b, ok := payload["balance"].(string); ok {
			answer.Balance = ParseMoney(b)
		}
	default:
		if kind, ok := payload["error"].(string); ok {
			answer.Refusal = RefusalKind(kind)
		}
		if id, ok := payload["account_id"].(string); ok {
			answer.NamedAccount = id
		}
	}
	return answer
}

// --- assertion-support reads ------------------------------------------

// LastRefusal is the SSOT every "Then the response is refused as X" step
// reads (world.go § lastRefusal doc comment on the World struct).
func (w *World) LastRefusal() RefusalKind { return w.lastRefusal }

// LastTenantAnswer exposes the most recent provisioning answer.
func (w *World) LastTenantAnswer() TenantAnswer { return w.lastTenantAnswer }

// LastAnswer exposes the most recent account/transfer answer.
func (w *World) LastAnswer() Answer { return w.lastAnswer }

// LastBooks exposes the most recent verdict.
func (w *World) LastBooks() BooksReport { return w.lastBooks }

// LastOpenPair exposes the last two OpenAccount answers, for "both accounts
// are opened successfully".
func (w *World) LastOpenPair() (Answer, Answer) {
	n := len(w.openAnswers)
	if n < 2 {
		return Answer{}, Answer{}
	}
	return w.openAnswers[n-2], w.openAnswers[n-1]
}

// TenantIsProvisioned reports whether a tenant name currently resolves to a
// credential — the durable record ProvisionTenant left behind, unaffected by
// any later, unrelated provisioning call.
func (w *World) TenantIsProvisioned(name TenantName) bool {
	_, ok := w.tenants[name]
	return ok
}

// TenantKey exposes a provisioned tenant's own credential, for the
// distinctness assertion.
func (w *World) TenantKey(name TenantName) string { return w.tenants[name].Key }

// AdminKey exposes the platform-admin credential, for the same assertion.
func (w *World) AdminKey() string { return w.adminKey }

// ReadAccountBalance re-reads an account's balance as its owning tenant — the
// Then-side verification read every "tenant X's account Y balance reads Z"
// step uses.
func (w *World) ReadAccountBalance(ctx context.Context, tenant TenantName, account AccountName) (Money, error) {
	return w.readBalance(ctx, tenant, account)
}

// --- Then assertions ---------------------------------------------------
//
// None of the assertions below is routed through statedelta.AssertStateDelta:
// that helper needs a testing.TB to call Fatalf on, and this suite's godog
// runner (suite_test.go) drives scenarios from TestMain rather than from a
// *testing.T subtest — identical precedent to
// tests/acceptance/ledgercore/ledger_assertions.go's own OPS-5 section.
// The universe-bound discipline (Mandate 8) is honoured directly instead:
// every assertion below compares exactly one declared, port-exposed
// observable against its expected value and states its own reason on
// mismatch. CaptureUniverse/Universe above remain the declared universe for
// the scenarios that read multiple observables back (the isolation-proof
// scenarios), even though the comparison itself is a hand-written diff
// rather than a literal AssertStateDelta call.

func (w *World) AssertTenantProvisioned(name TenantName) error {
	if w.lastTenantAnswer.Name != string(name) {
		return fmt.Errorf("expected the response to name tenant %q, got %q (raw: %s)", name, w.lastTenantAnswer.Name, w.lastTenantAnswer.Raw)
	}
	if w.lastTenantAnswer.TenantID == "" || w.lastTenantAnswer.TenantKey == "" {
		return fmt.Errorf("expected a distinct tenant_id and tenant_key, got id=%q key=%q", w.lastTenantAnswer.TenantID, w.lastTenantAnswer.TenantKey)
	}
	return nil
}

func (w *World) AssertTenantKeyDistinctFromAdmin() error {
	if w.lastTenantAnswer.TenantKey == w.adminKey {
		return fmt.Errorf("the issued tenant_key must not equal the platform-admin credential")
	}
	return nil
}

func (w *World) AssertTenantStillProvisioned(name TenantName) error {
	if !w.TenantIsProvisioned(name) {
		return fmt.Errorf("expected tenant %q to remain provisioned", name)
	}
	return nil
}

func (w *World) AssertRefused(kind RefusalKind) error {
	if w.lastRefusal != kind {
		return fmt.Errorf("expected the response to be refused as %q, got refusal=%q status=%d",
			kind, w.lastRefusal, w.lastStatus)
	}
	return nil
}

func (w *World) AssertTenantRefused(kind RefusalKind) error {
	if w.lastTenantAnswer.Refusal != kind {
		return fmt.Errorf("expected the tenant response to be refused as %q, got refusal=%q status=%d",
			kind, w.lastTenantAnswer.Refusal, w.lastTenantAnswer.Status)
	}
	return nil
}

func (w *World) AssertNoNewTenant() error {
	if w.lastTenantAnswer.Provisioned() {
		return fmt.Errorf("expected no tenant to be created, but the response reports one provisioned")
	}
	return nil
}

func (w *World) AssertRefusedGeneric() error {
	if w.lastRefusal == "" {
		return fmt.Errorf("expected the response to be refused, got status %d (raw: %s)", w.lastStatus, w.lastAnswer.Raw)
	}
	return nil
}

func (w *World) AssertBothAccountsOpened() error {
	first, second := w.LastOpenPair()
	if !first.Accepted || !second.Accepted {
		return fmt.Errorf("expected both accounts to open successfully, got first.accepted=%v second.accepted=%v", first.Accepted, second.Accepted)
	}
	return nil
}

func (w *World) AssertAccountBalance(ctx context.Context, tenant TenantName, account AccountName, want Money) error {
	got, err := w.ReadAccountBalance(ctx, tenant, account)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("expected tenant %q's account %q balance to read %s, got %s", tenant, account, want, got)
	}
	return nil
}

func (w *World) AssertVerdict(want Verdict) error {
	if w.lastBooks.Verdict != want {
		return fmt.Errorf("expected verdict %q, got %q (raw: %s)", want, w.lastBooks.Verdict, w.lastBooks.Raw)
	}
	return nil
}

func (w *World) AssertNoMentionOf(tenant TenantName) error {
	if strings.Contains(w.lastBooks.Raw, string(tenant)) {
		return fmt.Errorf("expected no mention of tenant %q, but the response contains it: %s", tenant, w.lastBooks.Raw)
	}
	return nil
}

func (w *World) AssertDriftNames(account AccountName) error {
	for _, row := range w.lastBooks.Drifted {
		if row.Account == account {
			return nil
		}
	}
	return fmt.Errorf("expected the drift listing to name %q, got %v (raw: %s)", account, w.lastBooks.Drifted, w.lastBooks.Raw)
}

func (w *World) AssertNoAccountOf(tenant TenantName) error {
	for account, owner := range w.lastAccountTenant {
		if owner != tenant {
			continue
		}
		if strings.Contains(w.lastBooks.Raw, string(account)) {
			return fmt.Errorf("expected no account belonging to tenant %q, but %q appears in the response", tenant, account)
		}
	}
	return nil
}

func (w *World) AssertBooksSucceeded() error {
	if w.lastBooks.Status != http.StatusOK {
		return fmt.Errorf("expected the verdict call to succeed with 200, got %d (raw: %s)", w.lastBooks.Status, w.lastBooks.Raw)
	}
	if w.lastBooks.Verdict == "" {
		return fmt.Errorf("expected a stated verdict, got none (raw: %s)", w.lastBooks.Raw)
	}
	return nil
}

func (w *World) AssertEntriesSucceeded() error {
	if !w.lastAnswer.Accepted {
		return fmt.Errorf("expected the entries call to succeed, got status %d (raw: %s)", w.lastAnswer.Status, w.lastAnswer.Raw)
	}
	return nil
}

func decodeBooksReport(status int, raw []byte, tenantScoped bool, tenantID string) BooksReport {
	report := BooksReport{Status: status, TenantScoped: tenantScoped, TenantID: tenantID, Raw: string(raw)}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	if verdict, ok := payload["verdict"].(string); ok {
		report.Verdict = ParseVerdict(verdict)
	}
	if kind, ok := payload["error"].(string); ok {
		report.Refusal = RefusalKind(kind)
	}
	if drifted, ok := payload["drifted"].([]any); ok {
		for _, d := range drifted {
			row, ok := d.(map[string]any)
			if !ok {
				continue
			}
			account, _ := row["account_id"].(string)
			report.Drifted = append(report.Drifted, DriftRow{Account: AccountName(account)})
		}
	}
	return report
}
