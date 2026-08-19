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

// RefusalKind is the sealed set of ways the ledger says no. It mirrors the
// domain's violation taxonomy (DDD-12) plus the two boundary refusals the
// adapter owns — a caller it cannot identify, and a request with no key.
//
// One step decorator covers every refusal because of this type. Adding a sixth
// refusal adds a constant here and a row in ParseRefusalKind, not a new step.
type RefusalKind string

const (
	UnknownAccount    RefusalKind = "unknown_account"
	InsufficientFunds RefusalKind = "insufficient_funds"
	InvalidAmount     RefusalKind = "invalid_amount"
	KeyConflict       RefusalKind = "idempotency_key_conflict"
	MissingKey        RefusalKind = "missing_idempotency_key"
	Unidentified      RefusalKind = "unidentified_caller"
)

// ParseRefusalKind coerces the Gherkin phrasing of a refusal into the taxonomy.
// The phrasings are the business-readable ones; the constants are the wire
// vocabulary the adapter emits.
func ParseRefusalKind(text string) RefusalKind {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "an unknown account":
		return UnknownAccount
	case "insufficient funds":
		return InsufficientFunds
	case "an invalid amount":
		return InvalidAmount
	case "a key conflict":
		return KeyConflict
	case "missing a key":
		return MissingKey
	case "unidentified":
		return Unidentified
	default:
		panic(fmt.Sprintf("unknown refusal %q — see RefusalKind in domain_types.go", text))
	}
}

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
