// handlers.go holds the real handler bodies for the driving routes this step
// wires. Encoding and status mapping only — the mutation itself is the
// application layer's job (internal/app), built at step 01-03
// (contract-shape: bounded-change, delegated).
package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

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
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.AccountID == "" {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		kind, ok := parseAccountKind(body.Type)
		if !ok {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		scope, _ := TenantScopeFromContext(r.Context())
		tenantID, _ := scope.Resolve()

		if err := ledger.CreateAccount(r.Context(), tenantID, body.AccountID, kind); err != nil {
			writeDomainError(w, r, err)
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

// provisionTenantRequest is the wire shape POST /tenants accepts.
type provisionTenantRequest struct {
	Name string `json:"name"`
}

// provisionTenantHandler mints a new tenant's identity and credential
// through the application shell (internal/app/usecases.go, step 01-03).
// requireOperatorKey (unmodified, DDD-22) already gates this route: only the
// platform-admin credential ever reaches this handler.
func provisionTenantHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body provisionTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.Name == "" {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		provisioned, err := ledger.ProvisionTenant(r.Context(), body.Name)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"tenant_id":  provisioned.TenantID,
			"name":       provisioned.Name,
			"tenant_key": provisioned.TenantKey,
		})
	}
}

// getBalanceHandler reads one account's stored balance.
func getBalanceHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := chi.URLParam(r, "id")

		scope, _ := TenantScopeFromContext(r.Context())
		tenantID, _ := scope.Resolve()

		account, err := ledger.GetBalance(r.Context(), tenantID, accountID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"account_id": account.ID(),
			"type":       string(account.Kind()),
			"balance":    formatMoney(account.Balance()),
		})
	}
}

// getEntriesHandler reads one account's ordered entry history, each row
// carrying the running balance it settled to — the trace that both proves two
// legs of a transfer settled together (US-5) and lets the operator see
// exactly where a drifted account parts company from its stored balance
// (milestone-05).
func getEntriesHandler(ledger *app.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := chi.URLParam(r, "id")

		// Dual mode (step 02-04): scope is exactly what requireTenantKeyOrOperatorKey
		// injected -- ScopedToTenant for a tenant_key caller, Unscoped() for the
		// platform OperatorKey -- and is passed straight through to GetEntries,
		// which forwards it unchanged to TransactionRepository.EntriesFor.
		scope, _ := TenantScopeFromContext(r.Context())

		traced, err := ledger.GetEntries(r.Context(), scope, accountID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"entries": entriesToWire(traced),
		})
	}
}

func entriesToWire(traced []app.TracedEntry) []map[string]any {
	wire := make([]map[string]any, 0, len(traced))
	for _, row := range traced {
		wire = append(wire, map[string]any{
			"transaction_id":  row.Entry.TransactionID(),
			"counterparty":    row.Entry.Counterparty(),
			"amount":          formatMoney(row.Entry.Amount()),
			"recorded_at":     row.Entry.RecordedAt().UTC(),
			"running_balance": formatMoney(row.RunningBalance),
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
//
// metrics observes the outcome at the single site it is decided (OPS-5,
// design decision 1): posted, rejected, or replayed, plus how long the
// request took to answer. The insufficient-funds branch additionally counts
// on its own dedicated series, since that refusal is the one operators watch
// for independently of the general rejection count.
func postTransferHandler(ledger *app.Ledger, metrics *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()

		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeRefusal(w, r, http.StatusBadRequest, "missing_idempotency_key", nil)
			return
		}

		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		// DisallowUnknownFields: a field the ledger does not know about is a
		// request it cannot read as a command, not a request it silently
		// tolerates (DDD-19).
		decoder := json.NewDecoder(bytes.NewReader(rawBody))
		decoder.DisallowUnknownFields()
		var body postTransferRequest
		if err := decoder.Decode(&body); err != nil {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}
		if body.From == "" || body.To == "" || body.Amount == "" {
			writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
			return
		}

		amount, err := parseAmount(body.Amount)
		if err != nil {
			if errors.Is(err, errAmountNotLexicallyANumber) {
				writeRefusal(w, r, http.StatusBadRequest, "malformed_request", nil)
				return
			}
			writeDomainError(w, r, err)
			return
		}

		scope, _ := TenantScopeFromContext(r.Context())
		tenantID, _ := scope.Resolve()

		result, err := ledger.PostTransfer(r.Context(), app.TransferRequest{
			From:           body.From,
			To:             body.To,
			Amount:         amount,
			IdempotencyKey: key,
			Fingerprint:    fingerprintTransfer(body.From, body.To, amount),
			TenantID:       tenantID,
		})
		if err != nil {
			if errors.Is(err, app.ErrIdempotencyKeyConflict) {
				writeRefusal(w, r, http.StatusConflict, "idempotency_key_conflict", nil)
				return
			}
			recordPostingFailure(metrics, err, time.Since(started))
			writeDomainError(w, r, err)
			return
		}

		status := recordPostingSuccess(metrics, result, time.Since(started))
		recordTransferLogFields(r.Context(), body, amount, key, result)

		writeJSON(w, status, transferAnswer(result))
	}
}

// recordPostingFailure observes the rejected outcome on the general posting
// series, plus the dedicated insufficient-funds series when that is the
// violation the ledger refused with (OPS-5, design decision 1). Extracted
// from postTransferHandler so the handler body reads as one sequence of
// decisions rather than metrics bookkeeping inlined into the error branch.
func recordPostingFailure(metrics *Metrics, err error, elapsed time.Duration) {
	metrics.ObservePosting(PostingRejected, elapsed)
	var violation domain.Violation
	if errors.As(err, &violation) && violation.Kind() == domain.InsufficientFunds {
		metrics.ObserveInsufficientFundsRejection()
	}
}

// recordPostingSuccess observes the posted-or-replayed outcome and returns
// the wire status that matches it (201 for a first posting, 200 for a
// replay). Kept alongside recordPostingFailure as the success-path
// counterpart of the same single decision site (OPS-5, design decision 1).
func recordPostingSuccess(metrics *Metrics, result app.Result, elapsed time.Duration) int {
	status := http.StatusCreated
	outcome := PostingPosted
	if result.Replayed {
		status = http.StatusOK
		outcome = PostingReplayed
	}
	metrics.ObservePosting(outcome, elapsed)
	if result.Replayed {
		metrics.ObserveIdempotentReplay()
	}
	return status
}

// recordTransferLogFields writes every fact discovered while posting a
// transfer onto the per-request log accumulator (OPS-5, design decision 5).
// Each value below is already known at its call site — none is re-derived.
// The raw idempotency key (`key`) is deliberately never passed here: only
// its hash is.
func recordTransferLogFields(ctx context.Context, body postTransferRequest, amount domain.Money, key string, result app.Result) {
	accumulator := fieldsFrom(ctx)
	accumulator.Set("transaction_id", result.Posting.Transaction.ID())
	accumulator.Set("account_ids", []string{body.From, body.To})
	accumulator.Set("amount_minor", amount.MinorUnits())
	accumulator.Set("currency", amount.Currency())
	accumulator.Set("idempotency_key_hash", hashIdempotencyKey(key))
	accumulator.Set("replayed", result.Replayed)
}

// hashIdempotencyKey computes the SHA-256 digest of the raw idempotency key,
// returning the full 64-character lowercase hex digest with no truncation
// (OPS-5, design decision 5). Pure function: input in, digest out, no side
// effects. The raw key itself must never reach fieldsFrom(...).Set(...)
// under any name, on any path — this is the sole legitimate use of the raw
// key besides the app-layer idempotency lookup it already feeds.
func hashIdempotencyKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
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
func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var violation domain.Violation
	if errors.As(err, &violation) {
		writeViolation(w, r, violation)
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": "internal_error",
	})
}

// writeRefusal answers a refusal that does not flow through writeViolation's
// exhaustive switch (malformed_request, missing_idempotency_key,
// idempotency_key_conflict) — it records violation_kind onto the per-request
// log accumulator itself, uniformly, at this single call site (OPS-5, design
// decision 2).
func writeRefusal(w http.ResponseWriter, r *http.Request, status int, kind string, extra map[string]any) {
	fieldsFrom(r.Context()).Set("violation_kind", kind)
	body := map[string]any{"error": kind}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// verdictHandler answers the operator's one question over the whole ledger by
// full scan (D9), through app.Ledger.VerifyBooks. It is the single handler
// wired to BOTH GET /health/trial-balance and GET /console/verdict
// (milestone-04, "the console and the health check give the operator the
// same answer") — sharing one function body is what guarantees the two
// surfaces can never drift apart, rather than two call sites independently
// reproducing the same verdict-before-figures rendering.
func verdictHandler(ledger *app.Ledger, metrics *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := ledger.VerifyBooks(r.Context())
		if err != nil {
			writeRefusal(w, r, http.StatusInternalServerError, "internal_error", nil)
			return
		}
		body := verdictBodyFor(report)

		// The three gauges/histogram below are set from data verdictBodyFor
		// already computed for the wire response — no value is re-derived
		// (OPS-5, design decision 1).
		metrics.SetTrialBalanceImbalance(body.ImbalanceMinor)
		metrics.ObserveTrialBalanceScanDuration(time.Duration(report.ElapsedMillis) * time.Millisecond)
		metrics.SetDriftedAccounts(len(body.Drifted))

		writeJSON(w, http.StatusOK, body)
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
