package domain

import "fmt"

// ViolationKind is the sealed set of ways the rules say no (DDD-12). Go has no
// sum types, so the closed set is built by hand: the discriminant below plus an
// interface that can only be satisfied inside this package.
//
// Exhaustiveness is not compiler-checked, which is why `golangci-lint` runs the
// `exhaustive` linter over every switch on this type in CI job 1 (brief.md
// § Functional modeling decisions — the compensating control).
type ViolationKind string

const (
	// Unbalanced — the entries of a transaction do not sum to zero (I1). This
	// guards the Post/Transaction assembly against a defect in the rulebook
	// itself; no caller input can reach it (ADR-008). Not a wire member.
	Unbalanced ViolationKind = "unbalanced"
	// InsufficientFunds — the movement would take a wallet below zero (I4).
	InsufficientFunds ViolationKind = "insufficient_funds"
	// UnknownAccount — the movement names an account that was never opened.
	UnknownAccount ViolationKind = "account_not_found"
	// AccountAlreadyExists — opening an account under an id that already
	// names one is refused (DDD-18). The identifier is already bound; a
	// caller retry cannot be told apart from a name collision between two
	// independent callers, so this is a refusal, never an idempotent success.
	AccountAlreadyExists ViolationKind = "account_already_exists"
	// InvalidAmount — the movement moves nothing, moves a negative amount, or
	// names a currency/scale the ledger cannot hold.
	InvalidAmount ViolationKind = "invalid_amount"
	// CurrencyMismatch —
	//
	// Unreachable through the driving ports today: every account is opened in
	// the ledger's single configured currency, so Post never sees two. That is
	// deliberate — this member is how multi-currency transactions, out of
	// scope is enforced rather than merely asserted, and I1 being per-currency
	// is why it is a domain refusal rather than validation. Reachable at layer
	// 1 now (PBT obligations, relax the same-currency assumption), and at
	// layer 3 the day POST /accounts accepts a currency. Do not delete for
	// being uncovered.
	CurrencyMismatch ViolationKind = "currency_mismatch"
	// TenantAlreadyExists — provisioning a tenant under a name that already
	// names one is refused (I10), mirroring AccountAlreadyExists (DDD-18) one
	// aggregate level up. The identifier is already bound; a caller retry
	// cannot be told apart from a name collision between two independent
	// callers, so this is a refusal, never an idempotent success.
	TenantAlreadyExists ViolationKind = "tenant_already_exists"
	// TenantNotFound — a lookup names a tenant that was never provisioned.
	// Cross-tenant account access does NOT use this member -- it reuses the
	// existing UnknownAccount member unchanged (I8): a tenant-scoped query
	// that cannot see another tenant's row returns nothing, which already
	// maps to account_not_found today. This is a deliberate domain-modelling
	// position, not an oversight.
	TenantNotFound ViolationKind = "tenant_not_found"
)

// Violation is the single domain error type. It keeps the idiomatic Go
// (value, error) shape so violations travel through errors.As and map onto a
// 422 without the HTTP adapter inventing its own error vocabulary.
type Violation struct {
	kind         ViolationKind
	account      string
	available    Money
	requested    Money
	fromCurrency string
	toCurrency   string
	tenant       string
	sealed       sealedViolation
}

// sealedViolation is unexported and has an unexported method, so no type
// outside this package can satisfy the taxonomy. That is what makes the set
// closed.
type sealedViolation interface{ sealedInDomain() }

// sealedMarker is the concrete value every constructor stamps into a
// Violation's sealed field.
type sealedMarker struct{}

func (sealedMarker) sealedInDomain() {}

// NewViolation builds a violation of the given kind, for members that carry no
// further detail.
func NewViolation(kind ViolationKind) Violation {
	return Violation{kind: kind, sealed: sealedMarker{}}
}

// NewInsufficientFunds builds the one violation that carries figures, because
// the caller needs both to explain the refusal to their user.
func NewInsufficientFunds(account string, available, requested Money) Violation {
	return Violation{
		kind:      InsufficientFunds,
		account:   account,
		available: available,
		requested: requested,
		sealed:    sealedMarker{},
	}
}

// NewUnknownAccount names the account that could not be found — a refusal with
// no name leaves the caller guessing.
func NewUnknownAccount(account string) Violation {
	return Violation{kind: UnknownAccount, account: account, sealed: sealedMarker{}}
}

// NewAccountAlreadyExists names the account whose id was already bound when
// opening was attempted (DDD-18).
func NewAccountAlreadyExists(account string) Violation {
	return Violation{kind: AccountAlreadyExists, account: account, sealed: sealedMarker{}}
}

// NewCurrencyMismatch names both currencies a movement tried to reconcile.
func NewCurrencyMismatch(fromCurrency, toCurrency string) Violation {
	return Violation{
		kind:         CurrencyMismatch,
		fromCurrency: fromCurrency,
		toCurrency:   toCurrency,
		sealed:       sealedMarker{},
	}
}

// NewTenantAlreadyExists names the tenant name that was already bound when
// provisioning was attempted (I10).
func NewTenantAlreadyExists(name string) Violation {
	return Violation{kind: TenantAlreadyExists, tenant: name, sealed: sealedMarker{}}
}

// NewTenantNotFound names the tenant that could not be found.
func NewTenantNotFound(tenantID string) Violation {
	return Violation{kind: TenantNotFound, tenant: tenantID, sealed: sealedMarker{}}
}

// Error satisfies the error interface.
func (v Violation) Error() string {
	switch v.kind {
	case Unbalanced:
		return "unbalanced: entries do not sum to zero"
	case InsufficientFunds:
		return fmt.Sprintf(
			"insufficient funds: account %s has %d available, %d requested",
			v.account, v.available.MinorUnits(), v.requested.MinorUnits(),
		)
	case UnknownAccount:
		return fmt.Sprintf("account not found: %s", v.account)
	case AccountAlreadyExists:
		return fmt.Sprintf("account already exists: %s", v.account)
	case InvalidAmount:
		return "invalid amount"
	case CurrencyMismatch:
		return fmt.Sprintf("currency mismatch: %s vs %s", v.fromCurrency, v.toCurrency)
	case TenantAlreadyExists:
		return fmt.Sprintf("tenant already exists: %s", v.tenant)
	case TenantNotFound:
		return fmt.Sprintf("tenant not found: %s", v.tenant)
	default:
		return string(v.kind)
	}
}

// Kind exposes the discriminant callers switch over.
func (v Violation) Kind() ViolationKind {
	return v.kind
}

// Account exposes the account a violation refers to, where it has one.
func (v Violation) Account() string {
	return v.account
}

// Available and Requested expose the shortfall figures on an insufficient-funds
// violation.
func (v Violation) Available() Money {
	return v.available
}

func (v Violation) Requested() Money {
	return v.requested
}

// FromCurrency and ToCurrency expose the two currencies a currency-mismatch
// violation could not reconcile.
func (v Violation) FromCurrency() string {
	return v.fromCurrency
}

func (v Violation) ToCurrency() string {
	return v.toCurrency
}

// Tenant exposes the tenant a violation refers to, where it has one.
func (v Violation) Tenant() string {
	return v.tenant
}
