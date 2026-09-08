// Package http, this file only: the test-only fault-injection seam (step
// 03-01). It mirrors postgres.AttemptOutOfBandChange's own "what the suite
// may never do for real, except through a named back door" precedent
// (internal/adapters/postgres/store.go) — a set of operations production
// code would never want and never calls, compiled into every binary
// unconditionally (there is no build tag anywhere in this codebase, and
// AttemptOutOfBandChange itself is not build-tag gated either — verified
// before choosing this shape), reachable only because production never
// supplies the one thing that unlocks it: AttemptOutOfBandChange needs the
// privileged DSN; this seam needs Deps.EnableTestOnlyFaultSeam set true,
// which cmd/api/main.go never does and has no flag/env var wired to do.
//
// The mechanism is HTTP, not a direct Go function call like
// AttemptOutOfBandChange's own, because this suite's World never holds a Go
// reference into the running server — every driving-port call in
// tests/acceptance/intertenanttransfer/world.go goes over the real HTTP
// server (world.go's own package doc), the same way a real caller would. A
// test-only HTTP endpoint, mounted only when Deps says so, is the one seam
// shape consistent with that suite's own "everything through HTTP" rule.
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// mountTestOnlyFaults wires the five endpoints
// tests/acceptance/intertenanttransfer/world.go's fault-injection methods
// call. Grouped under requireOperatorKey — the same platform-admin gate
// every other cross-cutting/administrative route in this router already
// uses — rather than left unauthenticated, since "test-only" is not a
// reason to skip the one auth check every other route in this file has.
func mountTestOnlyFaults(router chi.Router, deps Deps, ledger *app.Ledger, transferCoordinator *app.TransferCoordinator) {
	router.Group(func(testonly chi.Router) {
		testonly.Use(requireOperatorKey(deps.OperatorKey))

		testonly.Post("/testonly/faults/leg", injectLegFaultHandler(transferCoordinator))
		testonly.Post("/testonly/faults/reversal", injectReversalFaultHandler(transferCoordinator))
		testonly.Post("/testonly/faults/crash-forward", simulateCrashBeforeForwardLegHandler(transferCoordinator))
		testonly.Post("/testonly/faults/crash-reversal", simulateCrashBeforeReversalHandler(transferCoordinator))
		testonly.Post("/testonly/tick", runRetryTickerOnceHandler(transferCoordinator))
		testonly.Post("/testonly/seed-due", seedTransfersDueForRetryHandler(deps, ledger, transferCoordinator))
	})
}

type legFaultRequest struct {
	TransferID string `json:"transfer_id"`
	Leg        int    `json:"leg"`
	// FailCount (2026-09-08, DELIVER 03-04 back-propagation) is optional and
	// defaults to 1 (single one-shot fault, this endpoint's original
	// behavior) — set > 1 to fail that many CONSECUTIVE attempts before the
	// leg is allowed to succeed. See TransferCoordinator.InjectLegFaultCount.
	FailCount int `json:"fail_count"`
}

func injectLegFaultHandler(transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body legFaultRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed_request"})
			return
		}
		count := body.FailCount
		if count <= 0 {
			count = 1
		}
		transferCoordinator.InjectLegFaultCount(body.TransferID, body.Leg, count)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// injectReversalFaultHandler is injectLegFaultHandler's own reversal-side
// mirror (step 04-03) — forces the next `count` consecutive compensating-
// reversal Post attempts for the named transfer's leg-N reversal (leg 1 or
// 2) to fail, the seam a "the compensating reversal of leg N itself has
// failed on all 5 attempts of its own retry budget" Given step needs (World
// method + Given-step wiring in
// tests/acceptance/intertenanttransfer/{world.go,steps_intertenanttransfer_test.go}
// remain nw-acceptance-designer's own scope, per Amendment 3's own
// DISTILL-facing consequence note — not added here).
func injectReversalFaultHandler(transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body legFaultRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed_request"})
			return
		}
		count := body.FailCount
		if count <= 0 {
			count = 1
		}
		transferCoordinator.InjectReversalFaultCount(body.TransferID, body.Leg, count)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

type transferIDRequest struct {
	TransferID string `json:"transfer_id"`
}

func simulateCrashBeforeForwardLegHandler(transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body transferIDRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed_request"})
			return
		}
		transferCoordinator.SimulateCrashBeforeForwardLegAttempt(body.TransferID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func simulateCrashBeforeReversalHandler(transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body transferIDRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed_request"})
			return
		}
		transferCoordinator.SimulateCrashBeforeReversalAttempt(body.TransferID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// runRetryTickerOnceHandler single-steps processDueTransfers exactly once
// (RunRetryTickerOnce's own HTTP port) — no real wall-clock sleep needed
// anywhere in the acceptance suite to observe one tick's effect.
func runRetryTickerOnceHandler(transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claimed, err := transferCoordinator.ProcessDueTransfersOnce(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "tick_failed", "detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"claimed": claimed})
	}
}

type seedDueRequest struct {
	Count int `json:"count"`
}

// seedTransfersDueForRetryHandler seeds N synthetic, already-due
// transfer_state rows directly — the batch-limit scenario's own need (more
// due rows than one tick's batch cap). transfer_state.tenant_id and
// counterparty_tenant_id both carry a real foreign key to tenants (migration
// 0005/0006), so this provisions two throwaway tenants first, once per call,
// rather than naming tenant_ids nothing ever created. Each row is otherwise
// self-contained, deliberately bypassing SendTransfer's alias-resolution/
// account-bootstrap pipeline: this scenario asserts ClaimDue's own
// discovery-and-cap behavior, not settlement.
func seedTransfersDueForRetryHandler(deps Deps, ledger *app.Ledger, transferCoordinator *app.TransferCoordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body seedDueRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed_request"})
			return
		}

		sender, err := ledger.ProvisionTenant(r.Context(), "testonly-seed-sender-"+deps.IDGenerator())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "seed_failed", "detail": err.Error()})
			return
		}
		counterparty, err := ledger.ProvisionTenant(r.Context(), "testonly-seed-counterparty-"+deps.IDGenerator())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "seed_failed", "detail": err.Error()})
			return
		}

		amount, err := domain.NewMoney(1, "USD")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "seed_failed", "detail": err.Error()})
			return
		}
		now := deps.Clock()
		if now.IsZero() {
			now = time.Now()
		}
		for i := 0; i < body.Count; i++ {
			transferID := "xfr_testonly_seed_" + deps.IDGenerator()
			state := ports.TransferState{
				TransferID:           transferID,
				TenantID:             sender.TenantID,
				IdempotencyKey:       "testonly-seed-" + transferID,
				Status:               "retrying",
				Leg1Status:           "posted",
				Leg2Status:           "pending",
				Leg3Status:           "pending",
				NextAttemptAt:        now.Add(-time.Second),
				CounterpartyTenantID: counterparty.TenantID,
				TargetAccountID:      "settlement",
				Amount:               amount,
			}
			if err := transferCoordinator.SeedTransferStateDueNow(r.Context(), state); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "seed_failed", "detail": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"seeded": body.Count})
	}
}
