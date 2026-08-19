// Package http is the driving adapter: routing, API-key auth, and JSON
// encoding. It is the only place that knows about status codes, and it never
// invents its own error vocabulary — the sealed violation taxonomy (DDD-12)
// decides what a refusal is called.
//
// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
//
// The routes below are REAL and the middleware chain is REAL. Only the handler
// bodies are scaffolds, and they answer 501 rather than panicking. That is a
// deliberate choice for the RED gate: a panicking handler drops the connection
// and a scenario fails in transport, which classifies as BROKEN. Answering 501
// lets every scenario reach its Then and fail on the assertion, which is
// MISSING_FUNCTIONALITY — the only failure mode that makes GREEN meaningful
// later (nw-distill § Pre-DELIVER fail-for-the-right-reason gate).
//
// Registering the routes here also means the walking skeleton proves routing,
// argument handling, and the middleware chain from the first run: a scenario
// that got 404 where it expected 501 would be telling us the route is missing,
// which is a different and more useful failure than a silent one.
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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

	router.Post("/accounts", scaffold("create account"))
	router.Get("/accounts/{id}", scaffold("read balance"))
	router.Get("/accounts/{id}/entries", scaffold("trace entries"))
	router.Post("/transfers", scaffold("post transfer"))
	router.Get("/health/trial-balance", scaffold("verify books"))
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
