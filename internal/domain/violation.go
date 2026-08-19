// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
package domain

// ViolationKind is the sealed set of ways the rules say no (DDD-12). Go has no
// sum types, so the closed set is built by hand: the discriminant below plus an
// interface that can only be satisfied inside this package.
//
// Exhaustiveness is not compiler-checked, which is why `golangci-lint` runs the
// `exhaustive` linter over every switch on this type in CI job 1 (brief.md
// § Functional modeling decisions — the compensating control).
type ViolationKind string

const (
	// Unbalanced — the entries of a transaction do not sum to zero (I1).
	Unbalanced ViolationKind = "unbalanced"
	// InsufficientFunds — the movement would take a wallet below zero (I4).
	InsufficientFunds ViolationKind = "insufficient_funds"
	// UnknownAccount — the movement names an account that was never opened.
	UnknownAccount ViolationKind = "account_not_found"
	// InvalidAmount — the movement moves nothing, or moves a negative amount.
	InvalidAmount ViolationKind = "invalid_amount"
)

// Violation is the single domain error type. It keeps the idiomatic Go
// (value, error) shape so violations travel through errors.As and map onto a
// 422 without the HTTP adapter inventing its own error vocabulary.
type Violation struct {
	kind      ViolationKind
	account   string
	available Money
	requested Money
	sealed    sealedViolation
}

// sealedViolation is unexported and has an unexported method, so no type
// outside this package can satisfy the taxonomy. That is what makes the set
// closed.
type sealedViolation interface{ sealedInDomain() }

// NewViolation builds a violation of the given kind.
func NewViolation(kind ViolationKind) Violation {
	panic("NewViolation not yet implemented -- RED scaffold")
}

// NewInsufficientFunds builds the one violation that carries figures, because
// the caller needs both to explain the refusal to their user.
func NewInsufficientFunds(account string, available, requested Money) Violation {
	panic("NewInsufficientFunds not yet implemented -- RED scaffold")
}

// NewUnknownAccount names the account that could not be found — a refusal with
// no name leaves the caller guessing.
func NewUnknownAccount(account string) Violation {
	panic("NewUnknownAccount not yet implemented -- RED scaffold")
}

// Error satisfies the error interface.
func (v Violation) Error() string {
	panic("Violation.Error not yet implemented -- RED scaffold")
}

// Kind exposes the discriminant callers switch over.
func (v Violation) Kind() ViolationKind {
	panic("Violation.Kind not yet implemented -- RED scaffold")
}

// Account exposes the account a violation refers to, where it has one.
func (v Violation) Account() string {
	panic("Violation.Account not yet implemented -- RED scaffold")
}

// Available and Requested expose the shortfall figures on an insufficient-funds
// violation.
func (v Violation) Available() Money {
	panic("Violation.Available not yet implemented -- RED scaffold")
}

func (v Violation) Requested() Money {
	panic("Violation.Requested not yet implemented -- RED scaffold")
}
