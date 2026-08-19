// Package domain is the pure core: no I/O, no clock, no identifier generation.
// Every non-determinism arrives as a value (DDD-14). Types are immutable with
// unexported fields and smart constructors, so an illegal value cannot be built
// outside this package (DDD-15).
//
// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
// DELIVER replaces these bodies with the real implementation. Every scaffold
// panics rather than returning a zero value, so a half-finished implementation
// cannot make a scenario green by accident.
package domain

// Money is a signed amount in minor units (ADR-001 / DDD-5). Never a float:
// the whole point of a ledger is that the arithmetic is exact.
type Money struct {
	minorUnits int64
	currency   string
}

// NewMoney is the smart constructor. It rejects an unknown currency and any
// scale the ledger does not carry.
func NewMoney(minorUnits int64, currency string) (Money, error) {
	panic("NewMoney not yet implemented -- RED scaffold")
}

// MinorUnits exposes the amount for adapters that must render or persist it.
func (m Money) MinorUnits() int64 {
	panic("Money.MinorUnits not yet implemented -- RED scaffold")
}

// Currency exposes the currency for the per-currency balance rule (I1).
func (m Money) Currency() string {
	panic("Money.Currency not yet implemented -- RED scaffold")
}

// Add returns a new Money. There is no mutating form, by DDD-15.
func (m Money) Add(other Money) (Money, error) {
	panic("Money.Add not yet implemented -- RED scaffold")
}

// Negate returns the opposite amount, which is how the second leg of a
// movement is derived from the first.
func (m Money) Negate() Money {
	panic("Money.Negate not yet implemented -- RED scaffold")
}

// IsPositive reports whether the amount actually moves value. A transfer of
// zero or less moves nothing and is refused at the boundary.
func (m Money) IsPositive() bool {
	panic("Money.IsPositive not yet implemented -- RED scaffold")
}
