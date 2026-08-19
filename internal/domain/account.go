// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
package domain

// AccountKind is the taxonomy from the ubiquitous language. A wallet may never
// go negative (I4); a system account may, by design — it is the counterparty
// value enters the ledger from, and its negative balance records how much value
// exists.
type AccountKind string

const (
	Wallet AccountKind = "wallet"
	System AccountKind = "system"
)

// Account is an immutable snapshot of a holder of value. Applying a movement
// yields a new Account; there is no mutating method, so an Account holding an
// illegal balance is never produced (DDD-15).
type Account struct {
	id      string
	kind    AccountKind
	balance Money
}

// NewAccount is the smart constructor. It refuses a wallet opened with a
// negative balance, so I4 holds from the first moment the value exists.
func NewAccount(id string, kind AccountKind, balance Money) (Account, error) {
	panic("NewAccount not yet implemented -- RED scaffold")
}

// ID exposes the account identifier.
func (a Account) ID() string {
	panic("Account.ID not yet implemented -- RED scaffold")
}

// Kind exposes the taxonomy, which is what decides whether I4 applies.
func (a Account) Kind() AccountKind {
	panic("Account.Kind not yet implemented -- RED scaffold")
}

// Balance exposes the stored balance.
func (a Account) Balance() Money {
	panic("Account.Balance not yet implemented -- RED scaffold")
}

// Apply returns a new Account with the delta applied, or a violation when the
// result would take a wallet below zero. The I4 check happens on the way to
// constructing the value, never after.
func (a Account) Apply(delta Money) (Account, error) {
	panic("Account.Apply not yet implemented -- RED scaffold")
}
