// status.go is the ADR-008 wire mapping: the single exhaustive switch that
// translates a sealed domain.ViolationKind into the HTTP status and body
// fields the wire vocabulary promises. Status follows decision site: 404 the
// command named something absent, 409 the identifier supplied is already
// bound to something else, 422 the command was understood and the rules
// refuse it.
//
// Kept as its own file, one switch, so the `exhaustive` linter (wired at step
// 05-02) has a single clean site to check (brief.md § Functional modeling
// decisions — the compensating control for Go's lack of sum types).
package http

import (
	"net/http"

	"ledgerops/internal/domain"
)

// writeViolation maps a domain.Violation onto its wire status and body. It
// also records violation_kind onto the per-request log accumulator (OPS-5,
// design decision 2) at this single exhaustive-switch site — reusing the
// sealed classification rather than adding a second one elsewhere.
func writeViolation(w http.ResponseWriter, r *http.Request, v domain.Violation) {
	fieldsFrom(r.Context()).Set("violation_kind", string(v.Kind()))
	switch v.Kind() {
	case domain.UnknownAccount:
		writeRefusal(w, r, http.StatusNotFound, string(v.Kind()), map[string]any{
			"account_id": v.Account(),
		})
	case domain.AccountAlreadyExists:
		writeRefusal(w, r, http.StatusConflict, string(v.Kind()), map[string]any{
			"account_id": v.Account(),
		})
	case domain.InsufficientFunds:
		writeRefusal(w, r, http.StatusUnprocessableEntity, string(v.Kind()), map[string]any{
			"account_id": v.Account(),
			"available":  formatMoney(v.Available()),
			"requested":  formatMoney(v.Requested()),
		})
	case domain.InvalidAmount:
		writeRefusal(w, r, http.StatusUnprocessableEntity, string(v.Kind()), nil)
	case domain.CurrencyMismatch:
		writeRefusal(w, r, http.StatusUnprocessableEntity, string(v.Kind()), map[string]any{
			"from_currency": v.FromCurrency(),
			"to_currency":   v.ToCurrency(),
		})
	case domain.TenantAlreadyExists:
		writeRefusal(w, r, http.StatusConflict, string(v.Kind()), map[string]any{
			"tenant": v.Tenant(),
		})
	case domain.TenantNotFound:
		// Not reachable through a driving port yet -- POST /tenants (this
		// step) never looks a tenant up by id, so the courtesy-check branch
		// in ProvisionTenant only ever produces TenantAlreadyExists or nil.
		// Wired now so the `exhaustive` linter passes the moment
		// domain.TenantNotFound exists (it has, since step 01-02); the
		// caller that actually produces this refusal (trial-balance/
		// verify-tenant lookup) lands in step 03-01/03-02.
		writeRefusal(w, r, http.StatusNotFound, string(v.Kind()), map[string]any{
			"tenant": v.Tenant(),
		})
	case domain.Unbalanced:
		// Not a wire member (ADR-008): no caller input can reach it. If it
		// escapes anyway, that is a defect in the rulebook, not a refusal —
		// fall through to the internal-error branch below rather than
		// inventing a wire mapping for it.
		fallthrough
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}
