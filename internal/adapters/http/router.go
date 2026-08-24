// Package http is the driving adapter: routing, API-key auth, and JSON
// encoding. It is the only place that knows about status codes, and it never
// invents its own error vocabulary — the sealed violation taxonomy (DDD-12)
// decides what a refusal is called.
//
// POST /accounts, POST /transfers, and GET /accounts/{id} are real as of step
// 01-04, over the real application shell (internal/app) built at step 01-03.
// GET /health/trial-balance is real too — the walking skeleton reads it back
// to prove the ledger balances. GET /accounts/{id}/entries is real as of step
// 02-01, narrowly: it returns enough (transaction id, counterparty, amount,
// recorded-at) to prove a transfer's two legs settled under one transaction
// id; the full traceability wire shape, including running balance and
// unknown-account refusal, is milestone-05's job. GET /console/verdict and
// GET /metrics remain scaffolds: no active scenario exercises them yet.
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
func NewRouter(deps Deps) http.Handler {
	router := chi.NewRouter()
	router.Use(requireOperatorKey(deps.OperatorKey))

	ledger := app.NewLedger(deps.Store, deps.Clock, deps.IDGenerator)

	router.Post("/accounts", createAccountHandler(ledger))
	router.Get("/accounts/{id}", getBalanceHandler(ledger))
	router.Get("/accounts/{id}/entries", getEntriesHandler(ledger))
	router.Post("/transfers", postTransferHandler(ledger))
	router.Get("/health/trial-balance", trialBalanceHandler(ledger))
	router.Get("/console/verdict", scaffold("console verdict"))
	router.Get("/metrics", scaffold("metrics exposition"))

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
