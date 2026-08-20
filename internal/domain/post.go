package domain

import "time"

// Post is the posting rulebook (DDD-14) — the one pure function where I1 and I4
// are decided, and the primary target of the property suite and of nightly
// mutation testing.
//
// It performs no I/O, reads no clock, and generates no identifiers. The
// already-locked account snapshots, the current time, and the transaction id
// all arrive as values, so every test over it is deterministic and reproducible
// by seed. The application layer's remaining job is to lock, call this once,
// and persist what it returns.
//
// Contract, as the acceptance scenarios observe it through the driving ports:
//
//   - the returned entries always sum to zero, per currency (I1);
//   - a movement that would take a WALLET below zero returns an
//     InsufficientFunds violation carrying available and requested;
//   - a movement naming an account absent from snapshots returns
//     UnknownAccount naming it;
//   - a movement of a non-positive amount returns InvalidAmount;
//   - a SYSTEM account may go below zero, and this is a rule, not an oversight.
func Post(cmd TransferCommand, snapshots []Account, now time.Time, transactionID string) (Posting, error) {
	if !cmd.Amount.IsPositive() {
		return Posting{}, NewViolation(InvalidAmount)
	}

	fromAccount, ok := findAccount(snapshots, cmd.From)
	if !ok {
		return Posting{}, NewUnknownAccount(cmd.From)
	}
	toAccount, ok := findAccount(snapshots, cmd.To)
	if !ok {
		return Posting{}, NewUnknownAccount(cmd.To)
	}

	fromDelta := cmd.Amount.Negate()
	toDelta := cmd.Amount

	updatedFrom, err := fromAccount.Apply(fromDelta)
	if err != nil {
		return Posting{}, err
	}
	updatedTo, err := toAccount.Apply(toDelta)
	if err != nil {
		return Posting{}, err
	}

	fromEntry, err := NewEntry(transactionID, fromAccount.ID(), toAccount.ID(), fromDelta, now, 0)
	if err != nil {
		return Posting{}, err
	}
	toEntry, err := NewEntry(transactionID, toAccount.ID(), fromAccount.ID(), toDelta, now, 1)
	if err != nil {
		return Posting{}, err
	}
	entries := []Entry{fromEntry, toEntry}

	// Guards the assembly against a defect in the rulebook above; no caller
	// input can reach this branch (ADR-008 — Unbalanced is not a wire member).
	if !EntriesSumToZero(entries) {
		return Posting{}, NewViolation(Unbalanced)
	}

	transaction, err := NewTransaction(transactionID, now)
	if err != nil {
		return Posting{}, err
	}

	return Posting{
		Transaction: transaction,
		Entries:     entries,
		Deltas: []BalanceDelta{
			{AccountID: updatedFrom.ID(), Delta: fromDelta},
			{AccountID: updatedTo.ID(), Delta: toDelta},
		},
	}, nil
}

// findAccount looks up the account named by id among the already-locked
// snapshots Post was handed. It performs no I/O — the snapshots are values.
func findAccount(snapshots []Account, id string) (Account, bool) {
	for _, account := range snapshots {
		if account.ID() == id {
			return account, true
		}
	}
	return Account{}, false
}

// TransferCommand is the movement as the integrator asked for it: one call, not
// two legs (journey post-a-transfer, mental_model).
type TransferCommand struct {
	From   string
	To     string
	Amount Money
}

// Posting is the complete intended change: the transaction, its entries, and
// the balance deltas to apply. Returning all three together is what lets the
// shell persist an atomic result without re-deriving anything.
type Posting struct {
	Transaction Transaction
	Entries     []Entry
	Deltas      []BalanceDelta
}

// Transaction is the indivisible set of entries that sum to zero.
type Transaction struct {
	id         string
	recordedAt time.Time
}

// NewTransaction is the smart constructor.
func NewTransaction(id string, recordedAt time.Time) (Transaction, error) {
	return Transaction{id: id, recordedAt: recordedAt}, nil
}

// ID exposes the transaction identifier the answer names.
func (t Transaction) ID() string {
	return t.id
}

// RecordedAt exposes the instant the movement was stamped with.
func (t Transaction) RecordedAt() time.Time {
	return t.recordedAt
}

// Entry is one side of a movement — an account and a signed amount. Also called
// a leg. Entries have no identity outside their transaction, and once written
// they are never updated or deleted (D7).
type Entry struct {
	transactionID string
	accountID     string
	counterparty  string
	amount        Money
	recordedAt    time.Time
	sequence      int64
}

// NewEntry is the smart constructor.
func NewEntry(transactionID, accountID, counterparty string, amount Money, recordedAt time.Time, sequence int64) (Entry, error) {
	return Entry{
		transactionID: transactionID,
		accountID:     accountID,
		counterparty:  counterparty,
		amount:        amount,
		recordedAt:    recordedAt,
		sequence:      sequence,
	}, nil
}

// AccountID, Counterparty, Amount, RecordedAt and Sequence expose what an entry
// must show for an operator to read it without cross-referencing anything by
// hand (US-5).
func (e Entry) AccountID() string     { return e.accountID }
func (e Entry) Counterparty() string  { return e.counterparty }
func (e Entry) Amount() Money         { return e.amount }
func (e Entry) RecordedAt() time.Time { return e.recordedAt }

// Sequence is the tiebreaker that keeps ordering settled for two entries
// sharing a clock tick (US-5). A timestamp alone cannot do it.
func (e Entry) Sequence() int64 { return e.sequence }

// BalanceDelta is the change to apply to one account's stored balance. Stored
// rather than derived (DDD-7 / ADR-003) precisely so slice 04 has two
// representations that can disagree — that disagreement is the check.
type BalanceDelta struct {
	AccountID string
	Delta     Money
}

// EntriesSumToZero is I1 as a predicate over an entry list, per currency. It is
// checked before a Posting value is constructed, so an unbalanced transaction
// is never built in the first place.
func EntriesSumToZero(entries []Entry) bool {
	sumsByCurrency := make(map[string]int64)
	for _, entry := range entries {
		sumsByCurrency[entry.Amount().Currency()] += entry.Amount().MinorUnits()
	}
	for _, sum := range sumsByCurrency {
		if sum != 0 {
			return false
		}
	}
	return true
}
