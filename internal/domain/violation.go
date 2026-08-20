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
	// InvalidAmount — the movement moves nothing, moves a negative amount, or
	// names a currency/scale the ledger cannot hold.
	InvalidAmount ViolationKind = "invalid_amount"
	// CurrencyMismatch — the two legs of a movement do not share a currency,
	// so they can never sum to zero per currency for any amount (I1). Decided
	// in the domain core (Post / Money.Add) per ADR-008 / DDD-19. Currently
	// unreachable through any driving port — every account is opened in the
	// ledger's single configured currency, and that unreachability is the
	// mechanism by which "multi-currency transactions, out of scope" is
	// enforced. Declared here so I1's "per currency" phrasing stays honest and
	// so the PBT obligation "relax the same-currency assumption" over
	// domain.Post has somewhere to land.
	CurrencyMismatch ViolationKind = "currency_mismatch"
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

// NewCurrencyMismatch names both currencies a movement tried to reconcile.
func NewCurrencyMismatch(fromCurrency, toCurrency string) Violation {
	return Violation{
		kind:         CurrencyMismatch,
		fromCurrency: fromCurrency,
		toCurrency:   toCurrency,
		sealed:       sealedMarker{},
	}
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
	case InvalidAmount:
		return "invalid amount"
	case CurrencyMismatch:
		return fmt.Sprintf("currency mismatch: %s vs %s", v.fromCurrency, v.toCurrency)
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
