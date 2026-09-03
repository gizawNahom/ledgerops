// Package multitenancy holds the acceptance suite for the multitenancy
// feature. It is its own package, not an extension of tests/acceptance/
// ledgercore, because this feature needs its own composition root (multiple
// concurrent tenant identities against one server) and its own walking
// skeleton (feature-delta.md § Wave: DISCUSS / Wave decisions summary —
// "this feature's own — slice 01 alone is not sufficient").
//
// This file is the Mandate-12 domain types module: every domain noun that
// appears in the .feature files is expressed here once, as a type. Step
// definitions coerce captured text into these types and delegate to a
// composition-root method — a step body never branches on a string literal.
//
// Money and AccountKind are deliberately NOT redefined here. They are
// imported from tests/acceptance/ledgercore, this project's existing SSOT for
// those two domain concepts (Mandate-12 — one type system, not two competing
// ones for the same noun). Every other noun below is new to this feature.
package multitenancy

import (
	"fmt"
	"strings"

	"ledgerops/tests/acceptance/ledgercore"
)

// Money and AccountKind are re-exported under this package's own names so a
// step definition never has to spell the ledgercore import at the call site.
// This is a type alias, not a new type — ledgercore.ParseMoney and this
// package's ParseMoney interoperate freely.
type Money = ledgercore.Money
type AccountKind = ledgercore.AccountKind

const (
	Wallet = ledgercore.Wallet
	System = ledgercore.System
)

func ParseMoney(text string) Money             { return ledgercore.ParseMoney(text) }
func ParseAccountKind(text string) AccountKind { return ledgercore.ParseAccountKind(text) }

// TenantName is the name an operator provisions a tenant under
// ("Acme Wallet"). It is the handle every scenario uses to refer to a tenant
// — the suite resolves it to the tenant_id/tenant_key the system actually
// issued, the same way a scenario never spells out a generated transaction id
// (ledgercore's own convention).
type TenantName string

// AccountName is the name an account is opened under, scoped to whichever
// tenant opened it. Reused verbatim from ledgercore's own type would require
// importing an unexported field; declared fresh here because the two carry no
// shared behavior beyond being a labelled string (unlike Money/AccountKind,
// which carry real parsing logic worth not duplicating).
type AccountName string

// Caller selects which credential the next driving-port call presents. This
// is the multitenancy-specific identity axis DDD-22 introduces: the sealed
// two-constructor TenantScope PLUS the platform-admin credential PLUS the two
// negative cases every credential-bearing feature needs (ledgercore's own
// NoKey/UnissuedKey precedent, extended with "another tenant's own key" —
// which is not a negative case for THAT tenant, only for the one being
// impersonated).
type Caller struct {
	role   callerRole
	tenant TenantName
}

type callerRole string

const (
	adminRole    callerRole = "admin"
	tenantRole   callerRole = "tenant"
	noCredRole   callerRole = "none"
	unissuedRole callerRole = "unissued"
)

// PlatformAdmin is the existing OperatorKey, reused unmodified as the
// platform-admin credential (DDD-22, D8).
func PlatformAdmin() Caller { return Caller{role: adminRole} }

// AsTenant selects a previously-provisioned tenant's own credential.
func AsTenant(name TenantName) Caller { return Caller{role: tenantRole, tenant: name} }

// NoCredential presents no Authorization header at all.
func NoCredential() Caller { return Caller{role: noCredRole} }

// UnissuedCredential presents a bearer token that was never issued to anyone
// — distinguishes "wrong secret" from "no secret" (ledgercore's own
// UnissuedKey precedent, carried into this feature's own identity axis).
func UnissuedCredential() Caller { return Caller{role: unissuedRole} }

// ParseCaller coerces the Gherkin phrasing of "who is calling" that does not
// name a specific tenant (the two negative, non-tenant-specific cases).
func ParseCaller(text string) Caller {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "no caller credential is presented", "no credential":
		return NoCredential()
	case "an unissued credential", "a credential that was never issued":
		return UnissuedCredential()
	default:
		panic(fmt.Sprintf("unknown caller phrasing %q — see ParseCaller in domain_types.go", text))
	}
}

// RefusalKind is this feature's addition to the sealed wire vocabulary
// (DDD-25): two new members (tenant_already_exists, tenant_not_found) plus
// the two existing ledgercore members this feature's own scenarios reuse
// unchanged (account_not_found for cross-tenant reads — ADR-011;
// unidentified_caller for a non-admin/no/unissued credential).
type RefusalKind string

const (
	TenantAlreadyExists RefusalKind = "tenant_already_exists"
	TenantNotFound      RefusalKind = "tenant_not_found"
	UnknownAccount      RefusalKind = "account_not_found"
	Unidentified        RefusalKind = "unidentified_caller"
)

// ParseRefusalKind coerces the Gherkin phrasing of a refusal into the
// taxonomy. Phrasings are the business-readable ones; constants are the wire
// vocabulary the adapter emits (mirrors ledgercore's own ParseRefusalKind).
func ParseRefusalKind(text string) RefusalKind {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "already provisioned", "a tenant that already exists":
		return TenantAlreadyExists
	case "an unknown tenant", "an unprovisioned tenant":
		return TenantNotFound
	case "an unknown account":
		return UnknownAccount
	case "unidentified":
		return Unidentified
	default:
		panic(fmt.Sprintf("unknown refusal %q — see RefusalKind in domain_types.go", text))
	}
}

// TenantAnswer is what POST /tenants hands back — the observable the
// provisioning scenarios assert on, never an internal struct.
type TenantAnswer struct {
	Status      int
	TenantID    string
	Name        string
	TenantKey   string
	Refusal     RefusalKind
	NamedTenant string
	Raw         string
}

// Provisioned marks whether the call the answer belongs to produced a tenant.
func (a TenantAnswer) Provisioned() bool { return a.Status == 201 }

// Verdict is the operator's one-question answer, reused verbatim from
// ledgercore's own product-facing wording (journey verify-the-books, S2) —
// this feature changes what the verdict is scoped TO, never the sentence.
type Verdict = ledgercore.Verdict

const (
	BooksBalanceYes = ledgercore.BooksBalanceYes
	BooksBalanceNo  = ledgercore.BooksBalanceNo
)

func ParseVerdict(text string) Verdict { return ledgercore.ParseVerdict(text) }

// DriftRow is one line of a tenant-scoped drift listing — identical shape to
// ledgercore's own, declared fresh here so this package's BooksReport does
// not reach into ledgercore for a field-level type (Mandate-12 draws the
// reuse line at "has real logic worth not duplicating"; a plain data shape
// does not meet that bar the way Money's parser does).
type DriftRow struct {
	Account  AccountName
	Stored   Money
	Computed Money
	Delta    Money
}

// BooksReport is the operator's answer to "does this tenant's ledger add up".
type BooksReport struct {
	Status       int
	TenantScoped bool
	TenantID     string
	Verdict      Verdict
	Drifted      []DriftRow
	Refusal      RefusalKind
	Raw          string
}

