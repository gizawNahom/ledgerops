// handlers.go holds the real handler bodies for the driving routes this step
// wires. Encoding and status mapping only — the mutation itself is the
// application layer's job (internal/app), built at step 01-03
// (contract-shape: bounded-change, delegated).
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"ledgerops/internal/app"
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

// getEntriesHandler reads one account's ordered entry history — the trace
// that proves two legs of a transfer settled together (US-5, narrow slice:
// this step needs only the fields that prove atomicity; the full
// traceability wire shape, including running balance, is milestone-05's job).
func getEntriesHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := chi.URLParam(r, "id")

		entries, err := ledger.GetEntries(r.Context(), accountID)
		if err != nil {
			writeDomainError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"entries": entriesToWire(entries),
		})
	}
}

func entriesToWire(entries []domain.Entry) []map[string]any {
	wire := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		wire = append(wire, map[string]any{
			"transaction_id": entry.TransactionID(),
			"counterparty":   entry.Counterparty(),
			"amount":         formatMoney(entry.Amount()),
			"recorded_at":    entry.RecordedAt().UTC(),
		})
	}
	return wire
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

		// DisallowUnknownFields: a field the ledger does not know about is a
		// request it cannot read as a command, not a request it silently
		// tolerates (DDD-19).
		decoder := json.NewDecoder(bytes.NewReader(rawBody))
		decoder.DisallowUnknownFields()
		var body postTransferRequest
		if err := decoder.Decode(&body); err != nil {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.From == "" || body.To == "" || body.Amount == "" {
			writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		amount, err := parseAmount(body.Amount)
		if err != nil {
			if errors.Is(err, errAmountNotLexicallyANumber) {
				writeRefusal(w, http.StatusBadRequest, "malformed_request", nil)
				return
			}
			writeDomainError(w, err)
			return
		}

		result, err := ledger.PostTransfer(r.Context(), app.TransferRequest{
			From:           body.From,
			To:             body.To,
			Amount:         amount,
			IdempotencyKey: key,
			Fingerprint:    fingerprintTransfer(body.From, body.To, amount),
		})
		if err != nil {
			if errors.Is(err, app.ErrIdempotencyKeyConflict) {
				writeRefusal(w, http.StatusConflict, "idempotency_key_conflict", nil)
				return
			}
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

// fingerprintTransfer computes the idempotency fingerprint over the PARSED
// command, not the raw request bytes (DDD-8): two bodies naming the same
// From/To/Amount but reordered or respaced must fingerprint identically. Using
// the parsed domain.Money (minor units + currency) rather than the wire
// literal also absorbs any lexical variance in how the same amount was
// spelled.
func fingerprintTransfer(from, to string, amount domain.Money) string {
	return fmt.Sprintf("%s|%s|%d|%s", from, to, amount.MinorUnits(), amount.Currency())
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

// decimalLiteral is the LEXICAL shape a wire amount must have to be readable
// as a command at all (DDD-19): an optional sign, digits, a decimal point,
// digits. Whether the scale and magnitude those digits carry are legal is not
// this adapter's call — that is domain knowledge, decided by
// domain.NewMoneyFromDecimalLiteral.
var decimalLiteral = regexp.MustCompile(`^(-?)(\d+)\.(\d+)$`)

// errAmountNotLexicallyANumber marks a parseAmount failure that belongs to
// the HTTP adapter (malformed_request/400), as distinct from a domain
// refusal (invalid_amount/422) for an amount that parsed fine lexically but
// is illegal.
var errAmountNotLexicallyANumber = errors.New("amount is not lexically a decimal number")

// parseAmount reads the wire decimal ("50.00") into domain.Money. The
// lexical check (is this shaped like a decimal number?) happens here; the
// legality check (does its scale and magnitude fit the currency?) is
// domain.NewMoneyFromDecimalLiteral's call, per the purity boundary DDD-19
// draws.
func parseAmount(text string) (domain.Money, error) {
	match := decimalLiteral.FindStringSubmatch(text)
	if match == nil {
		return domain.Money{}, errAmountNotLexicallyANumber
	}
	negative := match[1] == "-"
	return domain.NewMoneyFromDecimalLiteral(negative, match[2], match[3], "USD")
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

func writeRefusal(w http.ResponseWriter, status int, kind string, extra map[string]any) {
	body := map[string]any{"error": kind}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// trialBalanceHandler answers the operator's one question over the whole
// ledger by full scan (D9), through app.Ledger.VerifyBooks — the same verdict
// the console surface reads (milestone-04, "the console and the health check
// give the operator the same answer").
func trialBalanceHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := ledger.VerifyBooks(r.Context())
		if err != nil {
			writeRefusal(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}
		writeJSON(w, http.StatusOK, verdictBodyFor(report))
	}
}

// driftWire is one drifted account as the operator reads it: named account,
// both balances, and the delta — never detection alone (KPI-4).
type driftWire struct {
	AccountID string `json:"account_id"`
	Stored    string `json:"stored"`
	Computed  string `json:"computed"`
	Delta     string `json:"delta"`
}

// verdictBody is the wire shape for both GET /health/trial-balance and
// GET /console/verdict. Field DECLARATION ORDER is load-bearing: Go's
// json.Marshal on a struct emits fields in the order they are declared, and
// the operator must read "verdict" before any figure (journey
// verify-the-books, S2) — VerdictPosition/FigurePosition below prove that
// ordering byte-for-byte rather than merely asserting it by construction.
type verdictBody struct {
	Verdict        string      `json:"verdict"`
	ImbalanceMinor int64       `json:"imbalance_minor"`
	EntryCount     int         `json:"entry_count"`
	ElapsedMillis  int         `json:"elapsed_ms"`
	Drifted        []driftWire `json:"drifted"`

	// Positions are computed over the body WITHOUT these two fields (see
	// verdictBodyFor) and only appended afterward — trailing fields cannot
	// shift the byte offsets of everything that precedes them.
	VerdictPosition int `json:"verdict_position"`
	FigurePosition  int `json:"first_figure_position"`
}

// verdictBodyFor renders a BooksReport onto the wire, then measures where in
// its OWN serialised bytes the verdict and the first figure land, so the
// acceptance suite can assert the ordering directly rather than trust it.
func verdictBodyFor(report app.BooksReport) verdictBody {
	verdict := "Books balance: NO"
	if report.Balanced {
		verdict = "Books balance: YES"
	}

	drifted := make([]driftWire, 0, len(report.Drifted))
	for _, d := range report.Drifted {
		drifted = append(drifted, driftWire{
			AccountID: d.AccountID,
			Stored:    formatMoney(d.Stored),
			Computed:  formatMoney(d.Computed),
			Delta:     formatMoney(d.Delta),
		})
	}

	body := verdictBody{
		Verdict:        verdict,
		ImbalanceMinor: report.TrialBalance.MinorUnits(),
		EntryCount:     report.EntryCount,
		ElapsedMillis:  report.ElapsedMillis,
		Drifted:        drifted,
	}

	rendered, err := json.Marshal(body)
	if err != nil {
		return body
	}
	body.VerdictPosition = bytes.Index(rendered, []byte(`"verdict"`))
	body.FigurePosition = bytes.Index(rendered, []byte(`"imbalance_minor"`))
	return body
}
