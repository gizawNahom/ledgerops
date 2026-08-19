package ledgercore

import (
	"context"
	"fmt"

	"ledgerops/internal/adapters/postgres"
)

// Given-side helpers. Every one of them reaches the system through the same
// driving ports a scenario's When uses — a precondition established by writing
// straight to the store would prove the store accepts data the API cannot
// produce, which is exactly the kind of green nobody should trust.
//
// The two exceptions are the corruption helpers in ledger_world.go, which must
// bypass the driving ports because their whole point is producing a state the
// application is structurally incapable of producing (OPS-10).

// GivenAccount opens an account of the stated kind.
func (l *Ledger) GivenAccount(ctx context.Context, name AccountName, kind AccountKind) error {
	return l.OpenAccount(ctx, name, kind)
}

// GivenFundedAccount opens a wallet and funds it from the system account, which
// is the only way value may enter the ledger (journey post-a-transfer, S2).
func (l *Ledger) GivenFundedAccount(ctx context.Context, name AccountName, amount Money) error {
	if err := l.OpenAccount(ctx, name, Wallet); err != nil {
		return err
	}
	return l.GivenFundedFrom(ctx, name, amount, "treasury")
}

// GivenFundedFrom funds an existing account from a named system account.
func (l *Ledger) GivenFundedFrom(ctx context.Context, name AccountName, amount Money, from AccountName) error {
	if err := l.SubmitTransfer(ctx, Transfer{
		From:   from,
		To:     name,
		Amount: amount,
		Key:    IdempotencyKey(fmt.Sprintf("seed-fund-%s", name)),
	}); err != nil {
		return err
	}
	return l.ThenTheTransferIsAccepted()
}

// GivenSettledTransfers builds a ledger carrying n completed movements between
// two wallets, which is the `populated` environment slices 04 and 05 need.
func (l *Ledger) GivenSettledTransfers(ctx context.Context, n int) error {
	if err := l.GivenFundedAccount(ctx, "alice", ParseMoney("1000.00")); err != nil {
		return err
	}
	if err := l.OpenAccount(ctx, "bob", Wallet); err != nil {
		return err
	}
	for i := 0; i < n-1; i++ {
		if err := l.SubmitTransfer(ctx, Transfer{
			From:   "alice",
			To:     "bob",
			Amount: ParseMoney("10.00"),
			Key:    IdempotencyKey(fmt.Sprintf("seed-txn-%d", i)),
		}); err != nil {
			return err
		}
		if err := l.ThenTheTransferIsAccepted(); err != nil {
			return err
		}
	}
	return nil
}

// GivenMovementsAgainst records n movements against one account, alternating
// direction so the running balance has something to say.
func (l *Ledger) GivenMovementsAgainst(ctx context.Context, account AccountName, n int) error {
	if err := l.GivenFundedAccount(ctx, account, ParseMoney("1000.00")); err != nil {
		return err
	}
	if err := l.OpenAccount(ctx, "bob", Wallet); err != nil {
		return err
	}
	for i := 0; i < n-1; i++ {
		from, to := account, AccountName("bob")
		if i%2 == 1 {
			from, to = "bob", account
		}
		if err := l.SubmitTransfer(ctx, Transfer{
			From:   from,
			To:     to,
			Amount: ParseMoney("10.00"),
			Key:    IdempotencyKey(fmt.Sprintf("seed-move-%s-%d", account, i)),
		}); err != nil {
			return err
		}
		if err := l.ThenTheTransferIsAccepted(); err != nil {
			return err
		}
	}
	return nil
}

// GivenTwoMovementsAtTheSameInstant forces a clock collision rather than
// hoping for one. FakeClock cannot model a real same-nanosecond landing, so
// the collision is manufactured — see the infrastructure policy for what that
// leaves untested.
func (l *Ledger) GivenTwoMovementsAtTheSameInstant(ctx context.Context, account AccountName) error {
	if err := l.FixClockAt("2026-08-18T09:00:00Z"); err != nil {
		return err
	}
	if err := l.GivenFundedAccount(ctx, account, ParseMoney("100.00")); err != nil {
		return err
	}
	if err := l.OpenAccount(ctx, "bob", Wallet); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if err := l.SubmitTransfer(ctx, Transfer{
			From:   account,
			To:     "bob",
			Amount: ParseMoney("10.00"),
			Key:    IdempotencyKey(fmt.Sprintf("seed-tick-%d", i)),
		}); err != nil {
			return err
		}
	}
	return nil
}

// KillPartwayThrough interrupts the process during a posting and leaves the
// store exactly as the interruption left it. This is the chaos demo, and the
// reason slice 01 is worth anything.
func (l *Ledger) KillPartwayThrough(ctx context.Context, transfer Transfer) error {
	return postgres.InterruptPostingMidWrite(ctx, l.appDSN, string(transfer.From), string(transfer.To), int64(transfer.Amount), string(transfer.Key))
}

// MigrateFromZero applies the whole migration set to a bare store.
func (l *Ledger) MigrateFromZero(ctx context.Context) error {
	if err := postgres.Migrate(ctx, l.privilegedDSN); err != nil {
		return err
	}
	return l.serve(ctx)
}

// ApplyNewestMigration applies the newest schema change over existing history,
// which is the case that actually exists in operation — a migration validated
// only against an empty store proves nothing about it.
func (l *Ledger) ApplyNewestMigration(ctx context.Context) error {
	return postgres.MigrateStep(ctx, l.privilegedDSN)
}

func postgresMigrationStatements() (map[string]string, error) {
	return postgres.MigrationStatements()
}
