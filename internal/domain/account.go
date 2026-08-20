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
	if kind == Wallet && balance.MinorUnits() < 0 {
		zero, _ := NewMoney(0, balance.Currency())
		return Account{}, NewInsufficientFunds(id, zero, balance.Negate())
	}
	return Account{id: id, kind: kind, balance: balance}, nil
}

// ID exposes the account identifier.
func (a Account) ID() string {
	return a.id
}

// Kind exposes the taxonomy, which is what decides whether I4 applies.
func (a Account) Kind() AccountKind {
	return a.kind
}

// Balance exposes the stored balance.
func (a Account) Balance() Money {
	return a.balance
}

// Apply returns a new Account with the delta applied, or a violation when the
// result would take a wallet below zero, or when the delta's currency does not
// match the account's own. The I4 check happens on the way to constructing the
// value, never after.
func (a Account) Apply(delta Money) (Account, error) {
	newBalance, err := a.balance.Add(delta)
	if err != nil {
		return Account{}, err
	}
	if a.kind == Wallet && newBalance.MinorUnits() < 0 {
		return Account{}, NewInsufficientFunds(a.id, a.balance, delta.Negate())
	}
	return Account{id: a.id, kind: a.kind, balance: newBalance}, nil
}
