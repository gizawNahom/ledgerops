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

	metrics := resolveMetrics(deps.Metrics)
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
		// confirmed 2026-09-07, slice 01). RED scaffold — reuses
		// requireOperatorKey verbatim as the platform-admin gate (§ For
		// Acceptance Designer: "OperatorKey only"), mirroring POST /tenants'
		// own precedent above. No new gating logic needed for the
		// unauthenticated/wrong-key/tenant-scoped refusal — it is already
		// covered by this group's existing middleware.
		protected.Post("/tenant-links", scaffold("authorize_tenant_pair"))
		protected.Delete("/tenant-links/{link_id}", scaffold("revoke_tenant_link"))
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
		tenantScoped.Post("/transfers", postTransferOrCrossTenantHandler(ledger, metrics))

		// POST /counterparties (slice 02). RED scaffold — tenant-key-only,
		// same group as /transfers/  /accounts above.
		tenantScoped.Post("/counterparties", scaffold("register_counterparty_alias"))
	})

	// GET /transfers/{transfer_id} (slice 02 functional, slice 05 hardened).
	// RED scaffold. Temporarily mounted behind the existing
	// requireTenantKeyOrOperatorKey middleware as a placeholder gate — this
	// is NOT the final authorization boundary: ADR-016 calls for a new,
	// per-resource requireTransferParty middleware (grants exactly the two
	// tenants named by the transfer_id being read, plus the operator), which
	// requireTenantKeyOrOperatorKey structurally cannot express (it grants
	// "any authenticated tenant", not "this transfer's two tenants" — see
	// brief.md § Inter-tenant transfer, "Dual-party authorization"). DELIVER
	// must replace this middleware, not just the handler body, before slice
	// 05's isolation scenarios can go GREEN.
	router.Group(func(transfers chi.Router) {
		transfers.Use(requireTenantKeyOrOperatorKey(deps.OperatorKey, resolveTenantKey))
		transfers.Get("/transfers/{transfer_id}", scaffold("get_transfer"))
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
