// Package intertenanttransfer holds the acceptance suite for the
// inter-tenant-transfer feature. Its own package, its own composition root
// (a saga spanning two tenant identities plus the platform's own reserved
// tenant, none of which multitenancy's or ledgercore's own World models),
// mirroring both existing suites' own "own package per feature" precedent.
//
// This file is the Mandate-12 domain types module: every domain noun that
// appears in the .feature files is expressed here once, as a type. Step
// definitions coerce captured text into these types and delegate to a
// composition-root method -- a step body never branches on a string literal.
//
// Money, AccountKind, TenantName, and Caller are deliberately NOT redefined
// here. Money/AccountKind come from tests/acceptance/ledgercore (this
// project's existing SSOT for those two concepts); TenantName/Caller come
// from tests/acceptance/multitenancy (this project's existing SSOT for tenant
// identity and the credential-selection axis) -- reused unmodified, exactly
// as multitenancy itself reused Money/AccountKind from ledgercore
// (Mandate-12: one type system per domain noun, not a third competing one).
package intertenanttransfer

import (
	"fmt"
	"strings"

	"ledgerops/tests/acceptance/ledgercore"
	"ledgerops/tests/acceptance/multitenancy"
)

type Money = ledgercore.Money
type AccountKind = ledgercore.AccountKind

const (
	Wallet = ledgercore.Wallet
	System = ledgercore.System
)

func ParseMoney(text string) Money             { return ledgercore.ParseMoney(text) }
func ParseAccountKind(text string) AccountKind { return ledgercore.ParseAccountKind(text) }

type TenantName = multitenancy.TenantName

// Caller selects which credential the next driving-port call presents.
// Declared fresh in THIS package, with exported fields, rather than reusing
// multitenancy.Caller: that type is deliberately opaque (unexported fields,
// DDD-22) in its own package, which is correct there but leaves no way for
// this suite's transport layer (world.go's authenticate) to recover which
// tenant a Caller names -- the exact information HTTP auth needs. Same
// domain concept as multitenancy's own axis (admin / tenant / no-credential
// / unissued-credential), expressed transparently here instead.
type Caller struct {
	Role   CallerRole
	Tenant TenantName
}

type CallerRole string

const (
	AdminRole    CallerRole = "admin"
	TenantRole   CallerRole = "tenant"
	NoCredRole   CallerRole = "none"
	UnissuedRole CallerRole = "unissued"
)

func PlatformAdmin() Caller           { return Caller{Role: AdminRole} }
func AsTenant(name TenantName) Caller { return Caller{Role: TenantRole, Tenant: name} }
func NoCredential() Caller            { return Caller{Role: NoCredRole} }
func UnissuedCredential() Caller      { return Caller{Role: UnissuedRole} }

// AccountName is the name an account is opened under, scoped to whichever
// tenant opened it -- declared fresh, mirroring multitenancy's own AccountName
// (a labelled string with no parsing logic worth sharing across packages).
type AccountName string

// AliasName is a tenant-scoped counterparty alias (D11) -- the only way one
// tenant addresses another's account. Declared fresh: it carries no parsing
// logic, only the domain meaning "resolved within the caller's own tenant
// namespace," which the composition root enforces, not this type.
type AliasName string

// IdempotencyKey is the caller-supplied key that makes a POST /transfers
// request-level-idempotent (US-2's own AC) -- distinct from the
// coordinator's own per-leg synthesized keys, which are never spelled out in
// a scenario (they are an internal mechanism, not an observable).
type IdempotencyKey string

// TransferStatus is the five-state lifecycle D10 makes observable from one
// GET /transfers/{id} -- StatusReversalFailed added by Amendment 3
// (design/wave-decisions.md) as a fifth, disjoint terminal value: a
// compensating reversal that itself exhausts its own retry budget is never
// folded into StatusReversed's reason string, so a caller checking only
// `status == "reversed"` cannot mistake an incomplete compensation for a
// completed one.
type TransferStatus string

const (
	StatusPending        TransferStatus = "pending"
	StatusSettled        TransferStatus = "settled"
	StatusRetrying       TransferStatus = "retrying"
	StatusReversed       TransferStatus = "reversed"
	StatusReversalFailed TransferStatus = "reversal_failed"
)

func ParseTransferStatus(text string) TransferStatus {
	return TransferStatus(strings.TrimSpace(strings.ToLower(text)))
}

// LegStatus is one leg's own status within a transfer, per transfer_state's
// own schema (brief.md § Coordinator state persistence).
type LegStatus string

const (
	LegPending  LegStatus = "pending"
	LegPosted   LegStatus = "posted"
	LegRetrying LegStatus = "retrying"
	LegReversed LegStatus = "reversed"
)

// LinkStatus is a TenantLink's own lifecycle (D9 -- standing until revoked).
type LinkStatus string

const (
	LinkActive  LinkStatus = "active"
	LinkRevoked LinkStatus = "revoked"
)

func ParseLinkStatus(text string) LinkStatus {
	return LinkStatus(strings.TrimSpace(strings.ToLower(text)))
}

// RefusalKind is this feature's addition to the sealed wire vocabulary,
// mirroring multitenancy's own RefusalKind precedent -- new members plus the
// existing ones this feature's own scenarios reuse unchanged.
type RefusalKind string

const (
	TenantLinkAlreadyExists RefusalKind = "tenant_link_already_exists"
	TenantLinkNotFound      RefusalKind = "tenant_link_not_found"
	CounterpartyNotFound    RefusalKind = "counterparty_not_found"
	// TransferNotFound is deliberately NOT a domain.ViolationKind member
	// (brief.md § Inter-tenant transfer, "confirmed not a sealed member") --
	// declared here anyway, at the acceptance layer, because DISTILL owns
	// its own explicit coverage for this wire mapping (no exhaustive linter
	// covers it).
	TransferNotFound RefusalKind = "transfer_not_found"
	TenantNotFound   RefusalKind = "tenant_not_found"
	Unidentified     RefusalKind = "unidentified_caller"
	InsufficientFund RefusalKind = "insufficient_funds"
)

func ParseRefusalKind(text string) RefusalKind {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "a tenant link that already exists":
		return TenantLinkAlreadyExists
	case "a tenant link that was not found", "a tenant link not found":
		return TenantLinkNotFound
	case "counterparty not found":
		return CounterpartyNotFound
	case "transfer not found":
		return TransferNotFound
	case "an unknown tenant":
		return TenantNotFound
	case "unidentified":
		return Unidentified
	case "insufficient funds":
		return InsufficientFund
	default:
		panic(fmt.Sprintf("unknown refusal %q -- see ParseRefusalKind in domain_types.go", text))
	}
}

// LinkAnswer is what POST /tenant-links (and its revocation sibling) hand
// back -- the observable slice 01's scenarios assert on.
type LinkAnswer struct {
	Status  int
	LinkID  string
	TenantA string
	TenantB string
	Status_ LinkStatus // "Status_" avoids colliding with the HTTP Status field above
	Refusal RefusalKind
	Raw     string
}

func (a LinkAnswer) Created() bool { return a.Status == 201 }

// AliasAnswer is what POST /counterparties hands back.
type AliasAnswer struct {
	Status  int
	Alias   AliasName
	Refusal RefusalKind
	Raw     string
}

func (a AliasAnswer) Registered() bool { return a.Status == 201 }

// LegView is one leg's observable shape within a TransferAnswer.
type LegView struct {
	Leg    int
	Status LegStatus
}

// TransferAnswer is what POST /transfers (cross-tenant variant) and
// GET /transfers/{id} both hand back -- the single observable every slice
// 02-05 scenario asserts on.
type TransferAnswer struct {
	Status     int
	TransferID string
	TxStatus   TransferStatus
	Reason     string
	Legs       []LegView
	Refusal    RefusalKind
	Raw        string
}

func (a TransferAnswer) Accepted() bool { return a.Status >= 200 && a.Status < 300 }

// Verdict is the operator's one-question "do the books balance" answer (I3),
// reused verbatim from ledgercore -- the same wire sentence multitenancy's
// own CheckTrialBalance already asserts on, not redeclared (Mandate-12).
type Verdict = ledgercore.Verdict

const BooksBalanceYes = ledgercore.BooksBalanceYes

// LegStatusOf returns the named leg's status, or "" if the response never
// named it.
func (a TransferAnswer) LegStatusOf(leg int) LegStatus {
	for _, l := range a.Legs {
		if l.Leg == leg {
			return l.Status
		}
	}
	return ""
}
