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
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
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

		// POST /tenants (multitenancy, DISTILL 2026-09-03, Mandate 7 RED
		// scaffold). Mounted under the existing operator-key group: DDD-22
		// reuses requireOperatorKey unmodified as the platform-admin gate, so
		// an unauthenticated/wrong-key caller already gets the correct
		// unidentified_caller refusal for free, and only the happy path
		// reaches the scaffold below. ProvisionTenant does not exist yet
		// (internal/app/usecases.go) — ledger-core's own scaffold() helper
		// (below) is reused verbatim rather than inventing a second RED
		// convention: a real HTTP response (501, __SCAFFOLD__ body) is what
		// keeps this an acceptance-test RED, not a BROKEN/undefined-route.
		protected.Post("/tenants", scaffold("provision_tenant"))

		protected.Post("/accounts", createAccountHandler(ledger))
		protected.Get("/accounts/{id}", getBalanceHandler(ledger))
		protected.Get("/accounts/{id}/entries", getEntriesHandler(ledger))
		protected.Post("/transfers", postTransferHandler(ledger, metrics))
		protected.Get("/health/trial-balance", verdictHandler(ledger, metrics))
		protected.Get("/console/verdict", verdictHandler(ledger, metrics))
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

// requireOperatorKey is the real middleware, not a scaffold: authentication is
// the one thing that must work before anything else is reachable, and the
// unauthenticated-caller scenarios assert it directly.
func requireOperatorKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := r.Header.Get("Authorization")
			if expected == "" || presented != "Bearer "+expected {
				fieldsFrom(r.Context()).Set("violation_kind", "unidentified_caller")
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": "unidentified_caller",
				})
				return
			}
			next.ServeHTTP(w, r)
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
