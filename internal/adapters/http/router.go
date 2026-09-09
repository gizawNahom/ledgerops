// Package http is the driving adapter: routing, API-key auth, and JSON
// encoding. It is the only place that knows about status codes, and it never
// invents its own error vocabulary — the sealed violation taxonomy (DDD-12)
// decides what a refusal is called.
//
// POST /accounts, POST /transfers, and GET /accounts/{id} are real as of step
// 01-04, over the real application shell (internal/app) built at step 01-03.
// GET /health/trial-balance is real too — the walking skeleton reads it back
// to prove the ledger balances. GET /accounts/{id}/entries is real as of step
// 02-01 and carries the full traceability wire shape as of milestone-05:
// transaction id, counterparty, amount, recorded-at, running balance, and
// unknown-account refusal. GET /console/verdict is real as of step 06-02: it
// shares verdictHandler with GET /health/trial-balance, so the two surfaces
// cannot disagree by construction. GET /metrics is real as of OPS-5 step
// 01-01: a Prometheus exposition over the seven declared series
// (internal/adapters/http/metrics.go). As of step 01-02 it is mounted
// unauthenticated, outside the protected group, and its collector set
// travels through Deps.Metrics like every other adapter the composition
// root constructs. GET /console and GET /console/* are wired as of
// ledger-core-console's DEVOPS wave (build-output wiring, resolved as an
// infra concern -- see feature-delta.md § Wave: DEVOPS / Build-output
// wiring): they serve the built SPA shell and its assets, deliberately
// outside the operator-API-key middleware group -- see console_static.go.
// As of multitenancy step 02-03, POST /accounts, POST /transfers, and GET
// /accounts/{id} move under a requireTenantKey-only group (DDD-23 Option
// C) -- the OperatorKey is never accepted there. requireOperatorKey,
// requireTenantKey, and requireTenantKeyOrOperatorKey all share the
// bearerToken/isOperatorKey primitives and the refuseUnidentifiedCaller
// response shape (DDD-22).
package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
)

// Deps is what the entrypoint hands the adapter. The two function-typed ports
// travel all the way through, which is why a test can pin time and identity
// without touching anything else.
type Deps struct {
	Store       ports.Store
	OperatorKey string
	Clock       func() time.Time
	IDGenerator func() string

	// TenantKeyResolver resolves a presented bearer token's hash to the
	// tenant it belongs to (step 01-03's PostgreSQL-backed
	// ports.TenantKeyResolver in production). Threaded through Deps the
	// same way every other adapter is -- the composition root owns
	// construction, never NewRouter itself.
	//
	// Falls back to an always-miss resolver when nil, mirroring the
	// Metrics/Logger fallbacks below: a caller that never wires a
	// resolver gets a consistent, safe 401 refusal on every tenant-key
	// route rather than a nil-function-call panic.
	TenantKeyResolver ports.TenantKeyResolver

	// Metrics is the Prometheus collector set (internal/adapters/http/metrics.go),
	// threaded through Deps the same way Clock/IDGenerator are (OPS-5 step
	// 01-02, design decision 1). Constructed once in cmd/api/main.go via
	// NewMetrics() and handed in here, rather than built as a local variable
	// inside NewRouter -- the composition root owns construction of every
	// adapter, including this one.
	//
	// Falls back to a fresh NewMetrics() when nil, mirroring the Logger
	// fallback below -- test doubles constructed before this field existed
	// (e.g. the acceptance suite's composition root) are unaffected.
	Metrics *Metrics

	// Logger receives structured per-request JSON records (OPS-5,
	// feature-delta.md § Wave: DEVOPS / Observability stack). requestLogger
	// (request_logging.go) wraps the whole router with it as of step 02-01 —
	// see design decision 3
	// (docs/feature/ledger-core/design/2026-08-27-ops-5-observability-design-decisions.md).
	//
	// Falls back to slog.Default() when nil, mirroring the Metrics fallback
	// above -- test doubles or callers constructed before this field existed
	// are unaffected.
	Logger *slog.Logger

	// EnableTestOnlyFaultSeam mounts the test-only fault-injection/crash-
	// simulation/ticker-tick routes (testonly_faults.go, step 03-01) —
	// false by default, and cmd/api/main.go never sets it. Mirrors
	// postgres.AttemptOutOfBandChange's own back-door precedent: the
	// mounting code is compiled into every binary unconditionally, exactly
	// like that function is, but no production composition root ever
	// flips this on. Only the acceptance suite's own composition root
	// (tests/acceptance/intertenanttransfer/world.go) does.
	EnableTestOnlyFaultSeam bool
}

// NewRouter builds the production router. Real routes, real auth middleware.
//
// The operator-API-key middleware is scoped to a group, not the whole
// router: every JSON endpoint -- including GET /console/verdict, the one the
// console SPA itself calls -- requires the key, but GET /console and its
// static assets (mounted by mountConsole below) deliberately do not, and
// neither does GET /metrics (OPS-5 step 01-02) -- a scrape target cannot
// easily present an operator bearer key, and Prometheus never will. The
// HTML shell has to load before any key can be presented; the key is
// enforced at the JSON boundary it actually protects.
func NewRouter(deps Deps) http.Handler {
	router := chi.NewRouter()

	ledger := app.NewLedger(deps.Store, deps.Clock, deps.IDGenerator)
	transferCoordinator := app.NewTransferCoordinator(ledger)

	metrics := resolveMetrics(deps.Metrics)
	// Step 04-03: wires ledgerops_transfer_reversal_failed_total to the one
	// coordinator hook that ever fires it — see TransferCoordinator's own
	// onReversalFailed doc comment for why this is a function-value hook
	// rather than an import (internal/app cannot import this package).
	transferCoordinator.SetReversalFailedObserver(metrics.ObserveReversalFailed)
	logger := resolveLogger(deps.Logger)
	resolveTenantKey := resolveTenantKeyResolver(deps.TenantKeyResolver)

	// OPS-5 (fix-ledger-core-observability, design decision 3): requestLogger
	// wraps the entire router -- registered at the top level, before any
	// grouping -- so every response (success, domain refusal, the
	// unidentified-caller 401, static asset serve, metrics scrape) passes
	// through it and gets exactly one log line. middleware.RequestID runs
	// first so requestLogger can read the id it assigns.
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger))

	// OPS-5 (fix-ledger-core-observability, design decision 1 & 3): GET
	// /metrics is mounted unauthenticated, outside the protected group --
	// confirmed 2026-08-26, mirroring the GET /console static-asset
	// precedent (console_static.go). A scrape target cannot easily present
	// an operator bearer key, and Prometheus never will.
	router.Get("/metrics", metrics.Handler().ServeHTTP)

	router.Group(func(protected chi.Router) {
		protected.Use(requireOperatorKey(deps.OperatorKey))

		// POST /tenants (multitenancy, real as of step 01-04). Mounted under
		// the existing operator-key group: DDD-22 reuses requireOperatorKey
		// unmodified as the platform-admin gate, so an unauthenticated/
		// wrong-key/tenant-scoped caller is already refused
		// unidentified_caller by its existing string-compare before the
		// handler below is ever reached — no new gating logic needed here.
		protected.Post("/tenants", provisionTenantHandler(ledger))

		protected.Get("/health/trial-balance", verdictHandler(ledger, metrics))
		protected.Get("/console/verdict", verdictHandler(ledger, metrics))

		// POST /tenant-links, DELETE /tenant-links/{link_id} (inter-tenant-transfer,
		// real as of step 01-04). Reuses requireOperatorKey verbatim as the
		// platform-admin gate (§ For Acceptance Designer: "OperatorKey
		// only"), mirroring POST /tenants' own precedent above. No new
		// gating logic needed for the unauthenticated/wrong-key/tenant-scoped
		// refusal — it is already covered by this group's existing
		// middleware, which runs before either handler body below.
		protected.Post("/tenant-links", authorizeTenantPairHandler(ledger, deps.Store))
		protected.Delete("/tenant-links/{link_id}", revokeTenantLinkHandler(ledger))
	})

	// DDD-23 Option C: POST /accounts, POST /transfers, and GET
	// /accounts/{id} move under a requireTenantKey-ONLY group -- the
	// OperatorKey is never accepted here, unlike GET /accounts/{id}/entries
	// below (which stays dual-mode via requireTenantKeyOrOperatorKey). The
	// handlers read the tenant_id/scope requireTenantKey injects into
	// context (step 02-04) instead of the legacyTenantID placeholder step
	// 02-02 left in usecases.go.
	router.Group(func(tenantScoped chi.Router) {
		tenantScoped.Use(requireTenantKey(resolveTenantKey))

		tenantScoped.Post("/accounts", createAccountHandler(ledger))
		tenantScoped.Get("/accounts/{id}", getBalanceHandler(ledger))
		// postTransferOrCrossTenantHandler discriminates on request-body
		// shape (inter-tenant-transfer, confirmed 2026-09-07, slice 02): a
		// body naming "counterparty_alias" is the new cross-tenant variant
		// (RED scaffold — TransferCoordinator.SendTransfer does not exist
		// yet), anything else is byte-identical to today's existing,
		// unmodified single-tenant postTransferHandler. This keeps the
		// existing route's already-shipped contract untouched while the new
		// variant is scaffolded, per brief.md § Driving ports ("no existing
		// port's byte-identical behavior changes").
		tenantScoped.Post("/transfers", postTransferOrCrossTenantHandler(ledger, transferCoordinator, metrics))

		// POST /counterparties (slice 02, real as of step 02-04) —
		// tenant-key-only, same group as /transfers / /accounts above.
		tenantScoped.Post("/counterparties", registerCounterpartyAliasHandler(ledger))
	})

	// GET /transfers/{transfer_id} (slice 02 functional, slice 05 hardened,
	// step 05-01). requireTransferParty (below) replaces the earlier
	// requireTenantKeyOrOperatorKey placeholder: ADR-016 requires a
	// per-resource gate (grants exactly the two tenants named by the
	// transfer_id being read, plus the operator), which
	// requireTenantKeyOrOperatorKey structurally cannot express (it grants
	// "any authenticated tenant", not "this transfer's two tenants" — see
	// brief.md § Inter-tenant transfer, "Dual-party authorization").
	router.Group(func(transfers chi.Router) {
		transfers.Use(requireTransferParty(deps.OperatorKey, resolveTenantKey, deps.Store))
		transfers.Get("/transfers/{transfer_id}", getTransferHandler(transferCoordinator))
	})

	// GET /accounts/{id}/entries stays dual-mode (step 02-04): a tenant_key
	// caller sees only their own tenant's entries, and the existing
	// unscoped OperatorKey call -- the console's own credential -- keeps
	// working byte-identical to today (console-compatibility hard
	// constraint). requireTenantKeyOrOperatorKey tries the OperatorKey
	// comparison first, injecting Unscoped() on a match, and falls back to
	// the tenant resolver otherwise.
	router.Group(func(entries chi.Router) {
		entries.Use(requireTenantKeyOrOperatorKey(deps.OperatorKey, resolveTenantKey))

		entries.Get("/accounts/{id}/entries", getEntriesHandler(ledger))
	})

	mountConsole(router, consoleDistDir)

	// EnableTestOnlyFaultSeam mirrors postgres.AttemptOutOfBandChange's own
	// back-door precedent (internal/adapters/postgres/store.go): that
	// function is compiled into every build too, unconditionally, and is
	// unreachable from production only because production's own
	// composition root (cmd/api/main.go) never possesses the privileged
	// DSN it requires. There is no build tag anywhere in this codebase
	// (verified before choosing this shape) — the equivalent "credential
	// production never holds" here is this boolean, which cmd/api/main.go
	// never sets and has no flag/env var wired to set. Only
	// tests/acceptance/intertenanttransfer/world.go's own composition root
	// (serve(), via Deps) ever passes true. See testonly_faults.go.
	if deps.EnableTestOnlyFaultSeam {
		mountTestOnlyFaults(router, deps, ledger, transferCoordinator)
	}

	return router
}

// resolveMetrics returns deps' collector set, falling back to a fresh
// NewMetrics() when nil -- test doubles constructed before Deps.Metrics
// existed are unaffected. Mirrors resolveLogger below.
func resolveMetrics(deps *Metrics) *Metrics {
	if deps == nil {
		return NewMetrics()
	}
	return deps
}

// resolveLogger returns deps' logger, falling back to slog.Default() when
// nil -- mirrors resolveMetrics above.
func resolveLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

// resolveTenantKeyResolver returns deps' resolver, falling back to a
// resolver that never finds a match when nil -- mirrors resolveMetrics and
// resolveLogger above. A caller that has not wired step 01-03's real
// resolver yet (e.g. a composition root still mid-migration) gets a
// consistent 401 refusal on every tenant-key route instead of a
// nil-function-call panic.
func resolveTenantKeyResolver(resolve ports.TenantKeyResolver) ports.TenantKeyResolver {
	if resolve == nil {
		return func(context.Context, string) (string, bool, error) { return "", false, nil }
	}
	return resolve
}

// bearerToken extracts the presented bearer token from the Authorization
// header. Factored out of requireOperatorKey's original inline parse
// (DDD-22) so requireTenantKey and requireTenantKeyOrOperatorKey share
// exactly the same parsing behavior instead of each re-deriving it.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	presented := r.Header.Get("Authorization")
	if !strings.HasPrefix(presented, prefix) {
		return "", false
	}
	return strings.TrimPrefix(presented, prefix), true
}

// isOperatorKey reports whether a presented bearer token is the platform
// OperatorKey. Factored out of requireOperatorKey's original inline compare
// (DDD-22) -- an empty expected key never matches, exactly as before.
func isOperatorKey(presented, expected string) bool {
	return expected != "" && presented == expected
}

// hashBearerToken computes a presented bearer token's SHA-256 digest,
// hex-encoded -- the same digest/encoding shape ports.TenantKeyResolver
// expects for credentialHash (postgres.hashCredential hashes a tenant_key
// identically at provisioning time). Reuses handlers.go's hashIdempotencyKey
// rather than reimplementing the same three lines a third time in this
// package (DDD-26).
func hashBearerToken(token string) string {
	return hashIdempotencyKey(token)
}

// contextKey is a private type so a colliding string key elsewhere in the
// process cannot be read back as a tenant scope by accident (DDD-22: "typed
// context key, not a bare string").
type contextKey int

// tenantScopeContextKey is the sole key under which requireTenantKey and
// requireTenantKeyOrOperatorKey inject a caller's resolved ports.TenantScope.
// Reusing TenantScope itself (rather than a bare tenant_id string) means the
// value this middleware injects is exactly what EntriesFor/TrialBalance/
// ComputedBalances already accept -- step 02-04 threads it straight through
// with no translation.
const tenantScopeContextKey contextKey = iota

// withTenantScope returns a context carrying scope under the typed key.
func withTenantScope(ctx context.Context, scope ports.TenantScope) context.Context {
	return context.WithValue(ctx, tenantScopeContextKey, scope)
}

// TenantScopeFromContext reads back the scope a driving-port handler was
// authenticated under. Exported for the handlers step 02-04 wires -- this
// step only injects the value, reading it into a use-case call is that
// step's job.
func TenantScopeFromContext(ctx context.Context) (ports.TenantScope, bool) {
	scope, ok := ctx.Value(tenantScopeContextKey).(ports.TenantScope)
	return scope, ok
}

// refuseUnidentifiedCaller writes the one 401 refusal shape every auth
// middleware in this file answers with -- requireOperatorKey,
// requireTenantKey, and requireTenantKeyOrOperatorKey all reuse this instead
// of hand-rolling a second JSON error shape (DDD-22).
func refuseUnidentifiedCaller(w http.ResponseWriter, r *http.Request) {
	fieldsFrom(r.Context()).Set("violation_kind", "unidentified_caller")
	writeJSON(w, http.StatusUnauthorized, map[string]any{
		"error": "unidentified_caller",
	})
}

// requireOperatorKey is the real middleware, not a scaffold: authentication is
// the one thing that must work before anything else is reachable, and the
// unauthenticated-caller scenarios assert it directly.
func requireOperatorKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _ := bearerToken(r)
			if !isOperatorKey(token, expected) {
				refuseUnidentifiedCaller(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireTenantKey resolves the presented bearer token via a
// ports.TenantKeyResolver and injects the resolved tenant's scope into
// context; it never accepts the platform OperatorKey (DDD-23 Option C).
// Refuses 401 unidentified_caller -- the identical wire shape
// requireOperatorKey already answers with -- for a missing/malformed
// Authorization header, an unresolved token, or a resolver error alike: none
// of those are a caller this middleware can identify.
func requireTenantKey(resolve ports.TenantKeyResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				refuseUnidentifiedCaller(w, r)
				return
			}
			tenantID, found, err := resolve(r.Context(), hashBearerToken(token))
			if err != nil || !found {
				refuseUnidentifiedCaller(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(withTenantScope(r.Context(), ports.ScopedToTenant(tenantID))))
		})
	}
}

// requireTenantKeyOrOperatorKey tries the exact OperatorKey comparison
// first -- cheap, no I/O, byte-identical to requireOperatorKey's own check
// -- injecting the deliberate Unscoped() marker on a match so the route this
// guards stays byte-identical to today's admin-caller behavior, not merely
// similar. On no match it falls back to the same TenantKeyResolver lookup
// requireTenantKey performs. Refuses 401 unidentified_caller only when both
// fail.
func requireTenantKeyOrOperatorKey(operatorKey string, resolve ports.TenantKeyResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				refuseUnidentifiedCaller(w, r)
				return
			}
			if isOperatorKey(token, operatorKey) {
				next.ServeHTTP(w, r.WithContext(withTenantScope(r.Context(), ports.Unscoped())))
				return
			}
			tenantID, found, err := resolve(r.Context(), hashBearerToken(token))
			if err != nil || !found {
				refuseUnidentifiedCaller(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(withTenantScope(r.Context(), ports.ScopedToTenant(tenantID))))
		})
	}
}

// requireTransferParty admits a caller to GET /transfers/{transfer_id} only
// when the caller is the platform operator, or a tenant who is one of the
// transfer's own two parties (ADR-016). requireTenantKeyOrOperatorKey is
// structurally wrong here: it grants "any authenticated tenant", but a
// transfer names two SPECIFIC tenants -- an unrelated third tenant's own
// valid tenant_key must be refused even though it resolves successfully,
// which requireTenantKeyOrOperatorKey has no way to express (it has no
// notion of "this transfer's two tenants").
//
// Identifies the caller via the identical two-step check
// requireTenantKeyOrOperatorKey already performs -- isOperatorKey first,
// TenantKeyResolver fallback -- reusing bearerToken/isOperatorKey/
// hashBearerToken directly rather than reinventing token parsing (brief.md §
// Dual-party authorization).
//
// Reads the transfer row exactly ONCE (transferStateByID), regardless of
// what the identity check resolved, and funnels BOTH refusal reasons --
// transfer_id names no row at all, or transfer_id names a row neither party
// of which is the caller -- through the SAME refusal call
// (refuseTransferNotFound). There is no second, distinguishable "exists but
// forbidden" branch: no code path where a forbidden read costs an extra
// query, an extra branch, or a different log line a timing/behavioral
// side-channel could exploit.
func requireTransferParty(operatorKey string, resolve ports.TenantKeyResolver, store ports.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				refuseUnidentifiedCaller(w, r)
				return
			}

			isOperator := isOperatorKey(token, operatorKey)
			var callerTenantID string
			if !isOperator {
				tenantID, found, err := resolve(r.Context(), hashBearerToken(token))
				if err != nil || !found {
					refuseUnidentifiedCaller(w, r)
					return
				}
				callerTenantID = tenantID
			}

			transferID := chi.URLParam(r, "transfer_id")
			state, found, err := transferStateByID(r.Context(), store, transferID)
			if err != nil {
				writeDomainError(w, r, err)
				return
			}
			if !found {
				refuseTransferNotFound(w, r)
				return
			}
			if !isOperator && callerTenantID != state.TenantID && callerTenantID != state.CounterpartyTenantID {
				refuseTransferNotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// transferStateByID performs the one database read requireTransferParty ever
// issues per request -- a dedicated, read-only unit of work rolled back
// unconditionally since it never writes, mirroring
// resolveProvisionedTenantID's own shape (handlers.go). An absent
// transfer_id answers (zero value, false, nil), the same "no" shape
// TransferStateRepository.Get already returns -- not an error.
func transferStateByID(ctx context.Context, store ports.Store, transferID string) (ports.TransferState, bool, error) {
	uow, err := store.Begin(ctx)
	if err != nil {
		return ports.TransferState{}, false, err
	}
	defer uow.Rollback(ctx)

	return uow.TransferStates().Get(ctx, transferID)
}

// refuseTransferNotFound answers the one refusal shape requireTransferParty
// ever produces -- both the "no such transfer_id" and the "exists, but
// caller is neither party" cases funnel here (§ requireTransferParty's own
// doc comment).
func refuseTransferNotFound(w http.ResponseWriter, r *http.Request) {
	writeRefusal(w, r, http.StatusNotFound, "transfer_not_found", nil)
}

func scaffold(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error":     "__SCAFFOLD__",
			"operation": operation,
			"detail":    operation + " not yet implemented -- RED scaffold",
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
