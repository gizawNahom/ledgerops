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
//
// tenantID scopes the snapshot to the Tenant aggregate that owns it (I8).
// It is a required NewAccount constructor parameter — brief.md § Domain
// Model / Multitenancy: "tenant_id becomes a required field on the Account
// value type... there is no smart-constructor path that produces an Account
// without one." There is no wither and no default: every Account in memory
// carries the tenant it was constructed with, which is what lets
// domain.Post's I8 cross-check compare a TransferCommand's own tenant_id
// against a snapshot's tenant_id unconditionally, not only for callers that
// remembered to opt in.
type Account struct {
	id       string
	kind     AccountKind
	balance  Money
	tenantID string
}

// NewAccount is the smart constructor. It refuses a wallet opened with a
// negative balance, so I4 holds from the first moment the value exists.
// tenantID is required and positional, ahead of id — there is no path that
// constructs an Account without naming the tenant it belongs to (I8).
func NewAccount(tenantID, id string, kind AccountKind, balance Money) (Account, error) {
	if kind == Wallet && balance.MinorUnits() < 0 {
		zero, _ := NewMoney(0, balance.Currency())
		return Account{}, NewInsufficientFunds(id, zero, balance.Negate())
	}
	return Account{id: id, kind: kind, balance: balance, tenantID: tenantID}, nil
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

// TenantID exposes the tenant this snapshot is scoped to (I8).
func (a Account) TenantID() string {
	return a.tenantID
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
	return Account{id: a.id, kind: a.kind, balance: newBalance, tenantID: a.tenantID}, nil
}
