package ledgercore

import (
	"context"

	"github.com/cucumber/godog"
)

// Step definitions. Per Mandate-12 every body is a single statement that
// coerces its captured text into a domain type in argument position and
// delegates to a composition-root method. No branching, no loops, no business
// logic — the rules live in one place and both the suite and production read
// them from there.
//
// The decorator count stays low because the parameters are typed: one refusal
// step covers the whole sealed taxonomy, one account step covers the whole
// account taxonomy, one tamper step covers every role and action pair.

func RegisterSteps(ctx *godog.ScenarioContext, l *Ledger) {

	// --- Given: the ledger and its accounts --------------------------------

	ctx.Given(`^the ledger is running against an empty store$`, func(c context.Context) error {
		return l.StartAgainstEmptyStore(c)
	})

	ctx.Given(`^the store is a real PostgreSQL 16 instance with the schema migrated from zero$`, func(c context.Context) error {
		return l.StartAgainstEmptyStore(c)
	})

	ctx.Given(`^an empty store with no schema at all$`, func(c context.Context) error {
		return l.StartWithoutSchema(c)
	})

	ctx.Given(`^the service holds only the application credentials$`, func() error {
		l.ActAs(ApplicationRole)
		return nil
	})

	ctx.Given(`^a (wallet|system) account "([^"]*)" exists$`, func(c context.Context, kind, name string) error {
		return l.GivenAccount(c, AccountName(name), ParseAccountKind(kind))
	})

	ctx.Given(`^a wallet account "([^"]*)" funded with (\S+)$`, func(c context.Context, name, amount string) error {
		return l.GivenFundedAccount(c, AccountName(name), ParseMoney(amount))
	})

	ctx.Given(`^"([^"]*)" has been funded with (\S+) from "([^"]*)"$`, func(c context.Context, name, amount, from string) error {
		return l.GivenFundedFrom(c, AccountName(name), ParseMoney(amount), AccountName(from))
	})

	ctx.Given(`^a ledger carrying (\d+) settled transfers$`, func(c context.Context, n int) error {
		return l.GivenSettledTransfers(c, n)
	})

	ctx.Given(`^a wallet account "([^"]*)" with (\d+) movements recorded against it$`, func(c context.Context, name string, n int) error {
		return l.GivenMovementsAgainst(c, AccountName(name), n)
	})

	ctx.Given(`^two movements against "([^"]*)" recorded at the very same instant$`, func(c context.Context, name string) error {
		return l.GivenTwoMovementsAtTheSameInstant(c, AccountName(name))
	})

	// --- Given: who is calling, and with what clock ------------------------

	ctx.Given(`^the caller presents no operator key$`, func() error {
		l.ActAs(NoKey)
		return nil
	})

	ctx.Given(`^the caller presents an operator key that was never issued$`, func() error {
		l.ActAs(UnissuedKey)
		return nil
	})

	ctx.Given(`^the clock is fixed at "([^"]*)"$`, func(stamp string) error {
		return l.FixClockAt(stamp)
	})

	ctx.Given(`^the next generated identifier is "([^"]*)"$`, func(id string) error {
		l.FixNextIdentifier(id)
		return nil
	})

	// --- Given: deliberate damage, applied out of band ---------------------

	ctx.Given(`^the recorded amount of one entry belonging to "([^"]*)" is altered by (\S+) out of band$`,
		func(c context.Context, name, amount string) error {
			return l.CorruptEntry(c, AccountName(name), 1, ParseMoney(amount))
		})

	ctx.Given(`^the recorded amount of the third entry belonging to "([^"]*)" is altered by (\S+) out of band$`,
		func(c context.Context, name, amount string) error {
			return l.CorruptEntry(c, AccountName(name), 3, ParseMoney(amount))
		})

	ctx.Given(`^the stored balance of "([^"]*)" is altered by (\S+) out of band$`,
		func(c context.Context, name, amount string) error {
			return l.CorruptStoredBalance(c, AccountName(name), ParseMoney(amount))
		})

	// --- When: the integrator moves value ----------------------------------

	ctx.When(`^the integrator opens a wallet account "([^"]*)"$`, func(c context.Context, name string) error {
		return l.OpenAccount(c, AccountName(name), Wallet)
	})

	ctx.When(`^the integrator moves (\S+) from "([^"]*)" to "([^"]*)" under key "([^"]*)"$`,
		func(c context.Context, amount, from, to, key string) error {
			return l.SubmitTransfer(c, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: IdempotencyKey(key),
			})
		})

	ctx.When(`^the integrator moves (\S+) from "([^"]*)" to "([^"]*)" under no key$`,
		func(c context.Context, amount, from, to string) error {
			return l.SubmitTransfer(c, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: NoIdempotencyKey,
			})
		})

	ctx.When(`^the integrator moves an amount written as "([^"]*)" from "([^"]*)" to "([^"]*)" under key "([^"]*)"$`,
		func(c context.Context, amount, from, to, key string) error {
			return l.SubmitTransferWithAmountLiteral(c,
				AmountLiteral(amount), AccountName(from), AccountName(to), IdempotencyKey(key))
		})

	ctx.When(`^the integrator submits a transfer request (.+)$`, func(c context.Context, shape string) error {
		return l.SubmitMalformedTransfer(c, ParseMalformedPayload(shape))
	})

	ctx.When(`^the integrator repeats the same request under key "([^"]*)"$`, func(c context.Context, key string) error {
		return l.RepeatLastRequest(c, IdempotencyKey(key))
	})

	ctx.When(`^the integrator repeats the same request under key "([^"]*)" with its fields reordered and respaced$`,
		func(c context.Context, key string) error {
			return l.RepeatLastRequestRewritten(c, IdempotencyKey(key))
		})

	// --- When: the ledger is interrupted or restarted ----------------------

	ctx.When(`^the ledger is killed partway through moving (\S+) from "([^"]*)" to "([^"]*)"$`,
		func(c context.Context, amount, from, to string) error {
			return l.KillPartwayThrough(c, Transfer{
				From: AccountName(from), To: AccountName(to), Amount: ParseMoney(amount),
			})
		})

	ctx.When(`^the ledger is killed partway through moving (\S+) from "([^"]*)" to "([^"]*)" under key "([^"]*)"$`,
		func(c context.Context, amount, from, to, key string) error {
			return l.KillPartwayThrough(c, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: IdempotencyKey(key),
			})
		})

	ctx.When(`^the ledger is restarted against the same store$`, func(c context.Context) error {
		return l.Restart(c)
	})

	// --- When: many callers at once ----------------------------------------

	ctx.When(`^(\d+) integrators each move (\S+) from "([^"]*)" to "([^"]*)" at the same moment$`,
		func(c context.Context, n int, amount, from, to string) error {
			return l.RaceSpenders(c, n, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: "race",
			}, true)
		})

	ctx.When(`^(\d+) contended spends of (\S+) are attempted from "([^"]*)" to "([^"]*)"$`,
		func(c context.Context, n int, amount, from, to string) error {
			return l.RaceSpenders(c, n, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: "contended",
			}, true)
		})

	ctx.When(`^(\d+) integrators submit the same (\S+) transfer from "([^"]*)" to "([^"]*)" under key "([^"]*)" at the same moment$`,
		func(c context.Context, n int, amount, from, to, key string) error {
			return l.RaceSpenders(c, n, Transfer{
				From: AccountName(from), To: AccountName(to),
				Amount: ParseMoney(amount), Key: IdempotencyKey(key),
			}, false)
		})

	ctx.When(`^(\d+) movements from "([^"]*)" to "([^"]*)" and \d+ from "[^"]*" to "[^"]*" are attempted at the same moment$`,
		func(c context.Context, n int, a, b string) error {
			return l.RaceOpposingTransfers(c, n, AccountName(a), AccountName(b), ParseMoney("1.00"))
		})

	// --- When: the operator asks -------------------------------------------

	ctx.When(`^the operator asks whether the books balance$`, func(c context.Context) error {
		return l.AskWhetherBooksBalance(c, HealthSurface)
	})

	ctx.When(`^the operator asks whether the books balance on the (console|health) surface$`,
		func(c context.Context, surface string) error {
			return l.AskWhetherBooksBalance(c, ParseSurface(surface))
		})

	ctx.When(`^the operator traces "([^"]*)"$`, func(c context.Context, name string) error {
		return l.Trace(c, AccountName(name))
	})

	ctx.When(`^the operator follows the drift listing for "([^"]*)"$`, func(c context.Context, name string) error {
		return l.FollowDriftListing(c, AccountName(name))
	})

	// --- When: someone tries to rewrite history ----------------------------

	ctx.When(`^(the service's own|the privileged) credentials attempt to (.+)$`,
		func(c context.Context, who, action string) error {
			return l.AttemptTamper(c, ParseCredentials(who), ParseTamperAction(action))
		})

	// --- When: the schema moves --------------------------------------------

	ctx.When(`^the schema is migrated from zero$`, func(c context.Context) error {
		return l.MigrateFromZero(c)
	})

	ctx.When(`^the newest schema change is applied over that history$`, func(c context.Context) error {
		return l.ApplyNewestMigration(c)
	})

	// --- Then: how the submission was answered -----------------------------

	ctx.Then(`^the transfer is accepted$`, func() error {
		return l.ThenTheTransferIsAccepted()
	})

	ctx.Then(`^the account is created$`, func() error {
		return l.ThenTheAccountIsCreated()
	})

	ctx.Then(`^(?:the transfer|the trace|the second request|the caller|the account) is refused (?:as|for) (.+)$`,
		func(reason string) error {
			return l.ThenItIsRefusedAs(ParseRefusalKind(reason))
		})

	ctx.Then(`^the refusal names the account "([^"]*)"$`, func(name string) error {
		return l.ThenTheRefusalNamesTheAccount(AccountName(name))
	})

	ctx.Then(`^the refusal states (\S+) available against (\S+) requested$`, func(available, requested string) error {
		return l.ThenTheRefusalStatesTheShortfall(ParseMoney(available), ParseMoney(requested))
	})

	ctx.Then(`^the repeat is answered as a replay$`, func() error {
		return l.ThenTheRepeatIsAnsweredAsAReplay()
	})

	ctx.Then(`^both answers name the same transaction$`, func() error {
		return l.ThenBothAnswersNameTheSameTransaction()
	})

	ctx.Then(`^both answers are identical$`, func() error {
		return l.ThenBothAnswersAreIdentical()
	})

	ctx.Then(`^the legs in the replayed answer match the entries recorded for that transaction$`, func(c context.Context) error {
		return l.ThenTheReplayedLegsMatchTheRecordedEntries(c)
	})

	// --- Then: what the answer said ----------------------------------------

	ctx.Then(`^the answer names one transaction with two legs of (\S+) and (\S+)$`, func(first, second string) error {
		return l.ThenTheAnswerNamesOneTransactionWithLegs(ParseMoney(first), ParseMoney(second))
	})

	ctx.Then(`^the answer names the transaction "([^"]*)"$`, func(id string) error {
		return l.ThenTheAnswerNamesTheTransaction(id)
	})

	ctx.Then(`^both legs belong to the same transaction$`, func(c context.Context) error {
		return l.ThenBothLegsBelongToTheSameTransaction(c)
	})

	ctx.Then(`^the two legs sum to zero$`, func() error {
		return l.ThenTheTwoLegsSumToZero()
	})

	// --- Then: what the ledger holds ---------------------------------------

	ctx.Then(`^the balance of "([^"]*)" reads (\S+)$`, func(c context.Context, name, amount string) error {
		return l.ThenTheBalanceReads(c, AccountName(name), ParseMoney(amount))
	})

	ctx.Then(`^the balance of "([^"]*)" reads either (\S+) or (\S+)$`, func(c context.Context, name, first, second string) error {
		return l.ThenTheBalanceReadsEither(c, AccountName(name), ParseMoney(first), ParseMoney(second))
	})

	ctx.Then(`^the ledger (?:holds|still holds) (\d+) entries whose amounts sum to zero$`, func(c context.Context, n int) error {
		return l.ThenTheLedgerHoldsEntriesSummingToZero(c, n)
	})

	ctx.Then(`^the ledger holds no entries$`, func(c context.Context) error {
		return l.ThenTheLedgerHoldsEntriesSummingToZero(c, 0)
	})

	ctx.Then(`^every account's balance equals the sum of its own entries$`, func(c context.Context) error {
		return l.ThenEveryBalanceEqualsItsEntries(c)
	})

	ctx.Then(`^the ledger holds either both legs of that movement or neither$`, func(c context.Context) error {
		return l.ThenBothLegsOrNeitherSurvived(c)
	})

	ctx.Then(`^the ledger holds no entries whose amounts fail to sum to zero$`, func(c context.Context) error {
		return l.ThenBothLegsOrNeitherSurvived(c)
	})

	ctx.Then(`^"([^"]*)" is a (wallet|system) account$`, func(c context.Context, name, kind string) error {
		return l.ThenTheAccountIsOfKind(c, AccountName(name), ParseAccountKind(kind))
	})

	ctx.Then(`^the ledger is ready to accept a transfer$`, func(c context.Context) error {
		return l.ThenTheLedgerIsReadyToAcceptATransfer(c)
	})

	// --- Then: the operator's verdict --------------------------------------

	ctx.Then(`^the verdict (?:reads|still reads) "([^"]*)"$`, func(verdict string) error {
		return l.ThenTheVerdictReads(ParseVerdict(verdict))
	})

	ctx.Then(`^the verdict is stated before any per-account figures$`, func() error {
		return l.ThenTheVerdictPrecedesTheFigures()
	})

	ctx.Then(`^the trial balance is (\S+)$`, func(amount string) error {
		return l.ThenTheTrialBalanceIs(ParseMoney(amount))
	})

	ctx.Then(`^the verdict reports (\d+) entries scanned$`, func(n int) error {
		return l.ThenTheVerdictReportsEntriesScanned(n)
	})

	ctx.Then(`^the verdict reports how long the scan took$`, func() error {
		return l.ThenTheVerdictReportsHowLongItTook()
	})

	ctx.Then(`^both surfaces give the same verdict$`, func() error {
		return l.ThenBothSurfacesAgree()
	})

	ctx.Then(`^both surfaces report the same trial balance$`, func() error {
		return l.ThenBothSurfacesReportTheSameTrialBalance()
	})

	ctx.Then(`^"([^"]*)" is listed as drifted$`, func(name string) error {
		return l.ThenTheAccountIsListedAsDrifted(AccountName(name), true)
	})

	ctx.Then(`^"([^"]*)" is not listed as drifted$`, func(name string) error {
		return l.ThenTheAccountIsListedAsDrifted(AccountName(name), false)
	})

	ctx.Then(`^no account is listed as drifted$`, func() error {
		return l.ThenExactlyAccountsAreDrifted(0)
	})

	ctx.Then(`^exactly (\d+) accounts? (?:is|are) listed as drifted$`, func(n int) error {
		return l.ThenExactlyAccountsAreDrifted(n)
	})

	ctx.Then(`^the drift entry states the stored balance, the computed balance, and a delta of (\S+)$`, func(amount string) error {
		return l.ThenTheDriftEntryStatesTheDelta(ParseMoney(amount))
	})

	// --- Then: tracing ------------------------------------------------------

	ctx.Then(`^the entries are returned oldest first$`, func() error {
		return l.ThenTheEntriesAreOldestFirst()
	})

	ctx.Then(`^tracing "([^"]*)" a second time returns them in the very same order$`, func(c context.Context, name string) error {
		return l.ThenTheSecondTraceMatchesTheFirst(c, AccountName(name))
	})

	ctx.Then(`^each row carries a running balance$`, func() error {
		return l.ThenEachRowCarriesARunningBalance()
	})

	ctx.Then(`^the running balance on the last row reads the stored balance of "([^"]*)"$`, func(c context.Context, name string) error {
		return l.ThenTheFinalRunningBalanceReadsTheStoredBalance(c, AccountName(name))
	})

	ctx.Then(`^the running balance on the last row disagrees with the stored balance of "([^"]*)" by (\S+)$`,
		func(c context.Context, name, amount string) error {
			return l.ThenTheFinalRunningBalanceDisagreesBy(c, AccountName(name), ParseMoney(amount))
		})

	ctx.Then(`^the row where the running balance first parts company is the altered one$`, func() error {
		return l.ThenTheDivergingRowIsTheAlteredOne()
	})

	ctx.Then(`^every row names its transaction, its counterparty account, its amount, and when it happened$`, func() error {
		return l.ThenEveryRowIsLegible()
	})

	ctx.Then(`^no row names "([^"]*)" as its own counterparty$`, func(name string) error {
		return l.ThenNoRowIsItsOwnCounterparty(AccountName(name))
	})

	ctx.Then(`^no entries are returned$`, func() error {
		return l.ThenExactlyEntriesAreReturned(0)
	})

	ctx.Then(`^exactly (\d+) entr(?:y|ies) (?:is|are) returned$`, func(n int) error {
		return l.ThenExactlyEntriesAreReturned(n)
	})

	ctx.Then(`^the entries of "([^"]*)" are returned$`, func(name string) error {
		return l.ThenTheEntriesOfAreReturned(AccountName(name))
	})

	ctx.Then(`^every entry of that transaction is stamped "([^"]*)"$`, func(stamp string) error {
		return l.ThenEveryEntryIsStamped(stamp)
	})

	// --- Then: contended runs ----------------------------------------------

	ctx.Then(`^exactly (\d+) transfers? (?:is|are) accepted$`, func(n int) error {
		return l.ThenExactlyWereAccepted(n)
	})

	ctx.Then(`^exactly (\d+) transfers? (?:is|are) refused for (.+)$`, func(n int, reason string) error {
		return l.ThenExactlyWereRefusedFor(n, ParseRefusalKind(reason))
	})

	ctx.Then(`^no wallet balance was ever observed below zero$`, func() error {
		return l.ThenNoNegativeBalanceWasObserved()
	})

	ctx.Then(`^at least (\d+) (?:attempts|submissions) were made$`, func(n int) error {
		return l.ThenAtLeastAttemptsWereMade(n)
	})

	ctx.Then(`^exactly (\d+) transactions? (?:was|were) recorded for that key$`, func(c context.Context, n int) error {
		return l.ThenExactlyTransactionsWereRecordedForTheKey(c, n)
	})

	ctx.Then(`^exactly (\d+) pairs? of entries (?:was|were) recorded for that key$`, func(c context.Context, n int) error {
		return l.ThenExactlyEntryPairsWereRecordedForTheKey(c, n)
	})

	ctx.Then(`^all (\d+) answers name the same transaction$`, func(n int) error {
		return l.ThenAllAnswersNameTheSameTransaction(n)
	})

	ctx.Then(`^every attempt is answered$`, func() error {
		return l.ThenEveryAttemptWasAnswered()
	})

	ctx.Then(`^no attempt is answered with a deadlock$`, func() error {
		return l.ThenNoAttemptDeadlocked()
	})

	// --- Then: the store's own refusals ------------------------------------

	ctx.Then(`^the (?:alteration|erasure|attempt) is refused by the store$`, func() error {
		return l.ThenTheAttemptIsRefusedByTheStore()
	})

	ctx.Then(`^no migration in the set erases an entry$`, func(c context.Context) error {
		return l.ThenNoMigrationErasesAnEntry(c)
	})
}
