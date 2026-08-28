// Package ledgercore holds the acceptance suite for the ledger-core feature.
//
// This file is the Mandate-12 domain types module: every domain noun that
// appears in the .feature files is expressed here once, as a type. Step
// definitions coerce their captured text into these types in argument position
// and delegate to a composition-root method that takes the type — so a step
// body never branches on a string literal, and the DSL emerges from the type
// system rather than from a decorator per literal value.
package ledgercore

import (
	"fmt"
	"strconv"
	"strings"
)

// AccountKind is the account taxonomy from the ubiquitous language
// (brief.md § Ubiquitous language). The distinction is load-bearing: a wallet
// may never go negative (I4), a system account may, by design.
type AccountKind string

const (
	Wallet AccountKind = "wallet"
	System AccountKind = "system"
)

// ParseAccountKind coerces Gherkin text into the taxonomy. It panics on an
// unknown kind because that is a test-authoring error, not a run-time
// condition — failing at the step boundary points at the .feature line.
func ParseAccountKind(text string) AccountKind {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "wallet":
		return Wallet
	case "system":
		return System
	default:
		panic(fmt.Sprintf("unknown account kind %q — the taxonomy is wallet | system", text))
	}
}

// AccountName is the name an account is opened under in a scenario.
type AccountName string

// Money is a signed amount in minor units (ADR-001 / DDD-5). The suite carries
// minor units, never a float, for the same reason the production domain does.
type Money int64

// ParseMoney coerces a major-unit decimal from Gherkin ("50.00", "-1.00") into
// signed minor units. Two decimal places, which is the only scale slice 01-05
// exercise; a second currency scale is out of scope (DISCUSS § Out-of-scope).
func ParseMoney(text string) Money {
	text = strings.TrimSpace(text)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")

	whole, frac, found := strings.Cut(text, ".")
	if !found {
		frac = "00"
	}
	if len(frac) != 2 {
		panic(fmt.Sprintf("amount %q must carry exactly two decimal places", text))
	}
	units, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("amount %q is not a decimal amount: %v", text, err))
	}
	if negative {
		units = -units
	}
	return Money(units)
}

// String renders minor units back as a major-unit decimal, so assertion
// failures read in the same notation the .feature file uses.
func (m Money) String() string {
	sign := ""
	units := int64(m)
	if units < 0 {
		sign, units = "-", -units
	}
	return fmt.Sprintf("%s%d.%02d", sign, units/100, units%100)
}

// RefusalKind is the sealed set of ways the ledger says no — the whole wire
// vocabulary, across all three decision sites (DDD-17, ADR-008): the HTTP
// adapter, the application shell, and the pure core. DDD-12 sealed the core's
// own taxonomy; it never governed the boundary refusals the adapter owns.
//
// One step decorator covers every refusal because of this type. Adding a tenth
// refusal adds a constant here and a row in ParseRefusalKind, not a new step.
//
// Availability is deliberately absent. A store the ledger cannot reach is an
// outcome, not a refusal (DDD-20, ADR-009): admitting `service_unavailable`
// here would turn a taxonomy of things the rules say no to into a taxonomy of
// everything that can go wrong, and the sealed set would stop meaning anything.
type RefusalKind string

const (
	MalformedRequest     RefusalKind = "malformed_request"
	MissingKey           RefusalKind = "missing_idempotency_key"
	Unidentified         RefusalKind = "unidentified_caller"
	UnknownAccount       RefusalKind = "account_not_found"
	AccountAlreadyExists RefusalKind = "account_already_exists"
	KeyConflict          RefusalKind = "idempotency_key_conflict"
	InvalidAmount        RefusalKind = "invalid_amount"
	InsufficientFunds    RefusalKind = "insufficient_funds"
	CurrencyMismatch     RefusalKind = "currency_mismatch"
)

// ParseRefusalKind coerces the Gherkin phrasing of a refusal into the taxonomy.
// The phrasings are the business-readable ones; the constants are the wire
// vocabulary the adapter emits.
func ParseRefusalKind(text string) RefusalKind {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "a request that cannot be read":
		return MalformedRequest
	case "missing a key":
		return MissingKey
	case "unidentified":
		return Unidentified
	case "an unknown account":
		return UnknownAccount
	case "already open":
		return AccountAlreadyExists
	case "a key conflict":
		return KeyConflict
	case "an invalid amount":
		return InvalidAmount
	case "insufficient funds":
		return InsufficientFunds
	case "a currency mismatch":
		// Declared, and unreachable through the driving ports today: every
		// account is opened in the ledger's single configured currency and
		// nothing lets a caller ask for another one. That unreachability is
		// how "multi-currency, out of scope" is enforced rather than merely
		// asserted, so no scenario claims this phrasing. Its coverage sits at
		// layer 1 (§ PBT obligations, "relax the same-currency assumption").
		// The row exists so the day the API grows a currency field, the
		// scenario that needs it needs no new step. Do not delete for being
		// unused.
		return CurrencyMismatch
	default:
		panic(fmt.Sprintf("unknown refusal %q — see RefusalKind in domain_types.go", text))
	}
}

// MalformedPayload is the shape of a request the ledger cannot read as a
// command at all. Every member is decided at the HTTP adapter and answered
// `malformed_request` / 400 (DDD-19): the adapter parses only the *lexical*
// form of a request, so anything below never reaches the rules.
//
// It is a type rather than a literal per scenario for the usual Mandate-12
// reason — one step covers the whole shape space, and adding a shape adds a
// member and a row, not a decorator.
type MalformedPayload string

const (
	NotARequest      MalformedPayload = "not_a_request"
	AmountLeftOut    MalformedPayload = "amount_left_out"
	SourceLeftOut    MalformedPayload = "source_left_out"
	FieldNotKnown    MalformedPayload = "field_not_known"
	AmountNotANumber MalformedPayload = "amount_not_a_number"
	AmountLeftEmpty  MalformedPayload = "amount_left_empty"
	AmountBareNumber MalformedPayload = "amount_bare_number"
)

// ParseMalformedPayload coerces the Gherkin phrasing of a broken request.
func ParseMalformedPayload(text string) MalformedPayload {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "that is not a request at all":
		return NotARequest
	case "that leaves out the amount":
		return AmountLeftOut
	case "that leaves out the account it moves from":
		return SourceLeftOut
	case "that names a field the ledger does not know":
		return FieldNotKnown
	case "whose amount is not a number":
		return AmountNotANumber
	case "whose amount is left empty":
		return AmountLeftEmpty
	case "whose amount is sent as a bare number rather than written out":
		return AmountBareNumber
	default:
		panic(fmt.Sprintf("unknown malformation %q — see MalformedPayload in domain_types.go", text))
	}
}

// AmountLiteral is an amount exactly as the caller wrote it, unparsed.
//
// Money cannot carry these: an over-scale amount (50.001) or one beyond int64
// minor units is not representable by the type whose whole point is that it is
// exact, and ParseMoney rightly refuses to build one. The literal is how a
// scenario puts an amount to the ledger that the suite itself could not hold —
// which is the only honest way to ask what the ledger does with it (DDD-19:
// scale is a property of the currency, so legality is NewMoney's to decide,
// answered `invalid_amount` / 422 by the core rather than 400 by the adapter).
type AmountLiteral string

// Outcome is how a submitted transfer was answered.
type Outcome string

const (
	Accepted Outcome = "accepted"
	Replayed Outcome = "replayed"
	Refused  Outcome = "refused"
)

// Verdict is the operator's one-question answer (journey verify-the-books, S2).
// It is a sentence in the product, so it is a sentence here.
type Verdict string

const (
	BooksBalanceYes Verdict = "Books balance: YES"
	BooksBalanceNo  Verdict = "Books balance: NO"
)

// ParseVerdict coerces the quoted verdict sentence from Gherkin.
func ParseVerdict(text string) Verdict {
	switch strings.TrimSpace(text) {
	case string(BooksBalanceYes):
		return BooksBalanceYes
	case string(BooksBalanceNo):
		return BooksBalanceNo
	default:
		panic(fmt.Sprintf("unknown verdict %q — the ledger states YES or NO, in words", text))
	}
}

// Surface is the path the operator takes to the verdict. Both must give the
// same answer: the console must not be the only route to it
// (journey verify-the-books, error path S1).
type Surface string

const (
	ConsoleSurface Surface = "console"
	HealthSurface  Surface = "health"
)

// ParseSurface coerces the Gherkin surface name.
func ParseSurface(text string) Surface {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "console":
		return ConsoleSurface
	case "health":
		return HealthSurface
	default:
		panic(fmt.Sprintf("unknown surface %q — the verdict is reachable on console | health", text))
	}
}

// Credentials selects which role a step acts as. The split is what makes D7
// enforcement structural rather than conventional (OPS-10): the service holds
// ApplicationRole and can never rewrite history; only PrivilegedRole can, and
// the service never connects as it.
type Credentials string

const (
	ApplicationRole Credentials = "ledgerops_app"
	PrivilegedRole  Credentials = "ledgerops_migrate"
	NoKey           Credentials = "none"
	UnissuedKey     Credentials = "unissued"
)

// ParseCredentials coerces the Gherkin actor phrasing into a role.
func ParseCredentials(text string) Credentials {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "the service's own", "the service's own credentials", "application":
		return ApplicationRole
	case "the privileged", "the privileged credentials", "privileged":
		return PrivilegedRole
	default:
		panic(fmt.Sprintf("unknown credentials %q — see Credentials in domain_types.go", text))
	}
}

// TamperAction is what an out-of-band actor tries to do to recorded history.
// Every one of these must be refused for ApplicationRole; only the third is
// permitted for PrivilegedRole, and only so that slice 04 has a genuine drift
// to detect.
type TamperAction string

const (
	AlterEntry            TamperAction = "alter_entry"
	EraseEntry            TamperAction = "erase_entry"
	DisableProtection     TamperAction = "disable_protection"
	AlterStoredBalance    TamperAction = "alter_stored_balance"
	AlterEntryProtectedOn TamperAction = "alter_entry_protection_on"
)

// ParseTamperAction coerces the Gherkin phrasing of an out-of-band attempt.
func ParseTamperAction(text string) TamperAction {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "alter a recorded entry":
		return AlterEntry
	case "erase a recorded entry":
		return EraseEntry
	case "disable the append-only protection":
		return DisableProtection
	case "alter a recorded entry with the protection left on":
		return AlterEntryProtectedOn
	default:
		panic(fmt.Sprintf("unknown tamper action %q — see TamperAction in domain_types.go", text))
	}
}

// IdempotencyKey is the caller-owned retry key. The API must never generate one
// (journey post-a-transfer, shared_artifacts).
type IdempotencyKey string

// NoIdempotencyKey is the absent key — its own value rather than an empty
// string, so "the caller sent no key" and "the caller sent an empty key" stay
// distinguishable at the port.
const NoIdempotencyKey IdempotencyKey = "\x00absent"

// Transfer is a submitted movement, as the integrator thinks of it: one call,
// not two legs (journey post-a-transfer, mental_model).
type Transfer struct {
	From   AccountName
	To     AccountName
	Amount Money
	Key    IdempotencyKey
}

// Leg is one side of a movement as the answer reports it.
type Leg struct {
	Account AccountName
	Amount  Money
}

// Answer is what a driving port hands back for a submitted transfer. It is the
// observable the acceptance scenarios assert on — never an internal struct.
type Answer struct {
	Outcome       Outcome
	Status        int
	TransactionID string
	Legs          []Leg
	Refusal       RefusalKind
	NamedAccount  AccountName
	Available     Money
	Requested     Money
	Raw           string
}

// DriftRow is one line of the operator's drift listing: the account whose
// stored balance disagrees with its entries, and by how much. Attribution is
// asserted, not just detection — a bare NO would fail the operator (KPI-4).
type DriftRow struct {
	Account  AccountName
	Stored   Money
	Computed Money
	Delta    Money
}

// BooksReport is the operator's answer to "does this add up?".
type BooksReport struct {
	Verdict           Verdict
	TrialBalance      Money
	EntryCount        int
	ElapsedMillis     int
	Drifted           []DriftRow
	VerdictStatedAt   int
	FirstFigureStated int
}

// TraceRow is one row of an account's history with the running balance that
// makes a break point visible (journey verify-the-books, S4).
type TraceRow struct {
	TransactionID  string
	Counterparty   AccountName
	Amount         Money
	RecordedAt     string
	RunningBalance Money
}

// RaceReport is what a contended run reports back. The denominators are part of
// the observable on purpose: a harness that quietly ran ten iterations reports
// zero negatives and passes (kpi-contracts.yaml, KPI-2 note).
type RaceReport struct {
	Attempts                    int
	Accepted                    int
	RefusedInsufficientFunds    int
	Deadlocked                  int
	Answered                    int
	NegativeBalanceObservations int
	DistinctTransactionIDs      int
	EntryPairsStored            int
}

// --- OPS-5 observability (fix-ledger-core-observability, 2026-08-26) -------
//
// RCA: docs/analysis/2026-08-26-observability-status-false-claim-rca.md.
// DEVOPS decided OPS-5 (feature-delta.md § Wave: DEVOPS / Observability
// stack) — a Prometheus exposition at GET /metrics and structured per-request
// JSON logs — and the decision was never discharged: no scenario ever
// exercised it. These types are the vocabulary for the scenarios that close
// that gap.

// PostingResult is the `result` label OPS-5's postings counter carries. Sealed
// to the three shapes a posting attempt can settle into — accepted for the
// first time, refused, or answered again from the record (DDD-8).
type PostingResult string

const (
	PostedResult   PostingResult = "posted"
	RejectedResult PostingResult = "rejected"
	ReplayedResult PostingResult = "replayed"
)

// ParsePostingResult coerces the Gherkin phrasing of a counted outcome.
func ParsePostingResult(text string) PostingResult {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "posted":
		return PostedResult
	case "rejected":
		return RejectedResult
	case "replayed":
		return ReplayedResult
	default:
		panic(fmt.Sprintf("unknown posting result %q — see PostingResult in domain_types.go", text))
	}
}

// MetricSeries is the sealed set of Prometheus series OPS-5 declares
// (feature-delta.md § Wave: DEVOPS / Observability stack;
// kpi-contracts.yaml § runtime_instrumentation.metrics.series). One constant
// per series so a scenario names a series once, not as a repeated literal.
type MetricSeries string

const (
	PostingsTotal            MetricSeries = "ledgerops_postings_total"
	PostingDurationSeconds   MetricSeries = "ledgerops_posting_duration_seconds"
	InsufficientFundsTotal   MetricSeries = "ledgerops_insufficient_funds_rejections_total"
	IdempotentReplaysTotal   MetricSeries = "ledgerops_idempotent_replays_total"
	TrialBalanceImbalance    MetricSeries = "ledgerops_trial_balance_imbalance_minor"
	TrialBalanceScanDuration MetricSeries = "ledgerops_trial_balance_scan_duration_seconds"
	DriftAccounts            MetricSeries = "ledgerops_drift_accounts"
)

// DeclaredMetricSeries is the whole sealed set, for the one scenario that
// asserts the exposition names every series DEVOPS declared rather than
// picking one and hoping the rest are there too.
func DeclaredMetricSeries() []MetricSeries {
	return []MetricSeries{
		PostingsTotal, PostingDurationSeconds, InsufficientFundsTotal,
		IdempotentReplaysTotal, TrialBalanceImbalance, TrialBalanceScanDuration,
		DriftAccounts,
	}
}

// MetricsSnapshot is a parsed reading of the exposition text, keyed by series
// name plus an optional label fragment (e.g. `result="posted"`), so a before
// and after pair can be handed to statedelta.AssertStateDelta the same way
// CaptureUniverse's ledger snapshot already is.
type MetricsSnapshot map[string]float64

// MetricKey builds the snapshot key for a series and an optional label
// fragment. An empty fragment reads the series' bare value (a gauge with no
// labels, e.g. ledgerops_drift_accounts).
func MetricKey(series MetricSeries, labelFragment string) string {
	if labelFragment == "" {
		return string(series)
	}
	return string(series) + "{" + labelFragment + "}"
}

// LogLine is one decoded JSON log record, holding only the fields OPS-5
// declares (feature-delta.md § Wave: DEVOPS / Observability stack;
// kpi-contracts.yaml § runtime_instrumentation.logs). Never carries the raw
// Authorization header or the raw idempotency key — that is the one property
// the release-blocking scenario exists to hold the adapter to.
type LogLine struct {
	RequestID          string
	Route              string
	Status             int
	ElapsedMillis      float64
	HasElapsedMillis   bool
	TransactionID      string
	AccountIDs         []string
	AmountMinor        int64
	Currency           string
	IdempotencyKeyHash string
	HasReplayedField   bool
	Replayed           bool
	ViolationKind      string
	Raw                string
}
