// handlers.go holds the real handler bodies for the driving routes this step
// wires. Encoding and status mapping only — the mutation itself is the
// application layer's job (internal/app), built at step 01-03
// (contract-shape: bounded-change, delegated).
package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// createAccountRequest is the wire shape POST /accounts accepts. Both fields
// are required lexically — a missing or unrecognised one never reaches the
// domain, it is answered malformed_request here (DDD-19).
type createAccountRequest struct {
	AccountID string `json:"account_id"`
	Type      string `json:"type"`
}

// createAccountHandler opens an account through the application shell. A
// wallet starts at zero (ADR-008); value enters only as a movement.
func createAccountHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.AccountID == "" {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		kind, ok := parseAccountKind(body.Type)
		if !ok {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		if err := ledger.CreateAccount(r.Context(), body.AccountID, kind); err != nil {
			writeDomainError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"account_id": body.AccountID,
			"type":       string(kind),
		})
	}
}

func parseAccountKind(text string) (domain.AccountKind, bool) {
	switch domain.AccountKind(text) {
	case domain.Wallet, domain.System:
		return domain.AccountKind(text), true
	default:
		return "", false
	}
}

// getBalanceHandler reads one account's stored balance.
func getBalanceHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := chi.URLParam(r, "id")

		account, err := ledger.GetBalance(r.Context(), accountID)
		if err != nil {
			writeDomainError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"account_id": account.ID(),
			"type":       string(account.Kind()),
			"balance":    formatMoney(account.Balance()),
		})
	}
}

// postTransferRequest is the wire shape POST /transfers accepts.
type postTransferRequest struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Amount string `json:"amount"`
}

// postTransferHandler moves value between two accounts. The Idempotency-Key
// header is required (ADR-005 / DDR-1) and threaded through to
// PostTransfer's idempotency plumbing; full replay/conflict handling is a
// later step's job (04-01/04-02) — this step reads and forwards the key.
func postTransferHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeRefusal(w, http.StatusBadRequest, "missing_idempotency_key", nil)
			return
		}

		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		var body postTransferRequest
		if err := json.Unmarshal(rawBody, &body); err != nil {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.From == "" || body.To == "" || body.Amount == "" {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		amount, err := parseAmount(body.Amount)
		if err != nil {
			writeRefusal(w, http.StatusUnprocessableEntity, string(domain.InvalidAmount), nil)
			return
		}

		result, err := ledger.PostTransfer(r.Context(), app.TransferRequest{
			From:           body.From,
			To:             body.To,
			Amount:         amount,
			IdempotencyKey: key,
			Fingerprint:    string(rawBody),
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}

		status := http.StatusCreated
		if result.Replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, transferAnswer(result))
	}
}

func transferAnswer(result app.Result) map[string]any {
	legs := make([]map[string]any, 0, len(result.Posting.Entries))
	for _, entry := range result.Posting.Entries {
		legs = append(legs, map[string]any{
			"account": entry.AccountID(),
			"amount":  formatMoney(entry.Amount()),
		})
	}
	return map[string]any{
		"transaction_id": result.Posting.Transaction.ID(),
		"legs":           legs,
	}
}

// parseAmount reads the wire decimal ("50.00") into domain.Money. It refuses
// anything that is not an exact two-decimal amount in the ledger's single
// configured currency, per ADR-001 / DDD-5 — no float ever exists here.
func parseAmount(text string) (domain.Money, error) {
	var whole, frac int64
	var negative bool
	rest := text
	if len(rest) > 0 && rest[0] == '-' {
		negative = true
		rest = rest[1:]
	}
	n, err := fmt.Sscanf(rest, "%d.%d", &whole, &frac)
	if err != nil || n != 2 {
		return domain.Money{}, domain.NewViolation(domain.InvalidAmount)
	}
	minor := whole*100 + frac
	if negative {
		minor = -minor
	}
	return domain.NewMoney(minor, "USD")
}

// formatMoney renders minor units as a major-unit decimal, matching the wire
// notation the driving adapters accept (ADR-001).
func formatMoney(m domain.Money) string {
	minor := m.MinorUnits()
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100)
}

// writeDomainError maps a domain.Violation onto the sealed HTTP status table
// (ADR-008). Any other error is an infrastructure failure the adapter did not
// cause and does not disguise as a domain refusal.
func writeDomainError(w http.ResponseWriter, err error) {
	var violation domain.Violation
	if errors.As(err, &violation) {
		writeViolation(w, violation)
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": "internal_error",
	})
}

func writeViolation(w http.ResponseWriter, v domain.Violation) {
	switch v.Kind() {
	case domain.UnknownAccount:
		writeRefusal(w, http.StatusNotFound, string(v.Kind()), map[string]any{
			"account_id": v.Account(),
		})
	case domain.InsufficientFunds:
		writeRefusal(w, http.StatusUnprocessableEntity, string(v.Kind()), map[string]any{
			"account_id": v.Account(),
			"available":  formatMoney(v.Available()),
			"requested":  formatMoney(v.Requested()),
		})
	case domain.InvalidAmount:
		writeRefusal(w, http.StatusUnprocessableEntity, string(v.Kind()), nil)
	case domain.CurrencyMismatch:
		writeRefusal(w, http.StatusUnprocessableEntity, string(v.Kind()), nil)
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

func writeRefusal(w http.ResponseWriter, status int, kind string, extra map[string]any) {
	body := map[string]any{"error": kind}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// trialBalanceHandler answers the operator's one question over the whole
// ledger by full scan (D9). It reads through the store directly rather than
// through app.Ledger.VerifyBooks — the drift/console verdict shape belongs to
// a later slice; this step needs only the trial balance the walking skeleton
// reads back.
func trialBalanceHandler(store ports.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uow, err := store.Begin(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = uow.Rollback(ctx)
			}
		}()

		total, count, err := uow.Transactions().TrialBalance(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		if err := uow.Commit(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		committed = true

		verdict := "Books balance: YES"
		if total.MinorUnits() != 0 {
			verdict = "Books balance: NO"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"verdict":         verdict,
			"imbalance_minor": total.MinorUnits(),
			"entry_count":     count,
		})
	}
}
