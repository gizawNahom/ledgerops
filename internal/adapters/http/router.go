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
// cannot disagree by construction. GET /metrics remains a scaffold: no active
// scenario exercises it yet. GET /console and GET /console/* are wired as of
// ledger-core-console's DEVOPS wave (build-output wiring, resolved as an
// infra concern -- see feature-delta.md § Wave: DEVOPS / Build-output
// wiring): they serve the built SPA shell and its assets, deliberately
// outside the operator-API-key middleware group -- see console_static.go.
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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
}

// NewRouter builds the production router. Real routes, real auth middleware.
//
// The operator-API-key middleware is scoped to a group, not the whole
// router: every JSON endpoint -- including GET /console/verdict, the one the
// console SPA itself calls -- requires the key, but GET /console and its
// static assets (mounted by mountConsole below) deliberately do not. The
// HTML shell has to load before any key can be presented; the key is
// enforced at the JSON boundary it actually protects.
func NewRouter(deps Deps) http.Handler {
	router := chi.NewRouter()

	ledger := app.NewLedger(deps.Store, deps.Clock, deps.IDGenerator)

	router.Group(func(protected chi.Router) {
		protected.Use(requireOperatorKey(deps.OperatorKey))

		protected.Post("/accounts", createAccountHandler(ledger))
		protected.Get("/accounts/{id}", getBalanceHandler(ledger))
		protected.Get("/accounts/{id}/entries", getEntriesHandler(ledger))
		protected.Post("/transfers", postTransferHandler(ledger))
		protected.Get("/health/trial-balance", verdictHandler(ledger))
		protected.Get("/console/verdict", verdictHandler(ledger))
		protected.Get("/metrics", scaffold("metrics exposition"))
	})

	mountConsole(router, consoleDistDir)

	return router
}

// requireOperatorKey is the real middleware, not a scaffold: authentication is
// the one thing that must work before anything else is reachable, and the
// unauthenticated-caller scenarios assert it directly.
func requireOperatorKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := r.Header.Get("Authorization")
			if expected == "" || presented != "Bearer "+expected {
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
