// Package domain is the pure core: no I/O, no clock, no identifier generation.
// Every non-determinism arrives as a value (DDD-14). Types are immutable with
// unexported fields and smart constructors, so an illegal value cannot be built
// outside this package (DDD-15).
package domain

// currencyScales lists the currencies this ledger can hold exactly, mapped to
// the number of minor-unit decimal places each uses (ISO 4217 exponent). A
// currency absent from this table has no legal scale here, so NewMoney refuses
// it as unknown — that refusal is also how "a scale the ledger cannot hold" is
// answered, since scale is a property of currency, not of the amount (DDD-19).
// Only one currency is in production scope for now; the table stays a table
// (not a single constant) so a second currency is a data change, not a code
// change.
var currencyScales = map[string]int{
	"USD": 2,
	"EUR": 2,
	"GBP": 2,
	"JPY": 0,
}

// Money is a signed amount in minor units (ADR-001 / DDD-5). Never a float:
// the whole point of a ledger is that the arithmetic is exact.
type Money struct {
	minorUnits int64
	currency   string
}

// NewMoney is the smart constructor. It rejects an unknown currency and any
// scale the ledger does not carry.
func NewMoney(minorUnits int64, currency string) (Money, error) {
	if _, known := currencyScales[currency]; !known {
		return Money{}, NewViolation(InvalidAmount)
	}
	return Money{minorUnits: minorUnits, currency: currency}, nil
}

// MinorUnits exposes the amount for adapters that must render or persist it.
func (m Money) MinorUnits() int64 {
	return m.minorUnits
}

// Currency exposes the currency for the per-currency balance rule (I1).
func (m Money) Currency() string {
	return m.currency
}

// Add returns a new Money. There is no mutating form, by DDD-15. Two amounts
// in different currencies cannot sum to zero for any value (I1), so adding
// across currencies is refused rather than silently coerced.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, NewCurrencyMismatch(m.currency, other.currency)
	}
	return Money{minorUnits: m.minorUnits + other.minorUnits, currency: m.currency}, nil
}

// Negate returns the opposite amount, which is how the second leg of a
// movement is derived from the first.
func (m Money) Negate() Money {
	return Money{minorUnits: -m.minorUnits, currency: m.currency}
}

// IsPositive reports whether the amount actually moves value. A transfer of
// zero or less moves nothing and is refused at the boundary.
// (smoke-test comment for the nightly mutation-delta CI job)
func (m Money) IsPositive() bool {
	return m.minorUnits > 0
}
