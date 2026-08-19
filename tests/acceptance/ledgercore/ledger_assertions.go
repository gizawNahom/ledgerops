package ledgercore

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// The Then-side of the composition root. Every assertion a scenario can make
// lives here as one method, so step bodies stay a single delegating call
// (Mandate-12 criterion 3) and the same vocabulary is reachable from any tier.
//
// Assertions read observables through the driving ports only. None of them
// reaches into the domain, the store, or an internal struct — a scenario that
// needed to would be asserting on something the operator cannot see, which is
// the coupling the Universe rule exists to prevent.

// --- how a submission was answered ----------------------------------------

// ThenTheTransferIsAccepted asserts the last submission posted.
func (l *Ledger) ThenTheTransferIsAccepted() error {
	if l.lastAnswer.Outcome == Accepted {
		return nil
	}
	return fmt.Errorf("expected the transfer to be accepted, but it was %s (answer: %s)",
		l.lastAnswer.Outcome, l.lastAnswer.Raw)
}

// ThenTheAccountIsCreated asserts the last account submission was accepted.
func (l *Ledger) ThenTheAccountIsCreated() error {
	return l.ThenTheTransferIsAccepted()
}

// ThenItIsRefusedAs asserts the last submission was refused for exactly this
// reason. One method covers the whole sealed taxonomy (DDD-12), which is why
// there is one refusal step rather than six.
func (l *Ledger) ThenItIsRefusedAs(kind RefusalKind) error {
	if l.lastAnswer.Outcome != Refused {
		return fmt.Errorf("expected a refusal for %s, but the request was %s (answer: %s)",
			kind, l.lastAnswer.Outcome, l.lastAnswer.Raw)
	}
	if l.lastAnswer.Refusal != kind {
		return fmt.Errorf("expected a refusal for %s, got %s (answer: %s)",
			kind, l.lastAnswer.Refusal, l.lastAnswer.Raw)
	}
	return nil
}

// ThenTheRefusalNamesTheAccount asserts the refusal says which account it
// could not find — a 404 with no name leaves the caller guessing.
func (l *Ledger) ThenTheRefusalNamesTheAccount(account AccountName) error {
	if l.lastAnswer.NamedAccount == account {
		return nil
	}
	return fmt.Errorf("expected the refusal to name %q, it named %q (answer: %s)",
		account, l.lastAnswer.NamedAccount, l.lastAnswer.Raw)
}

// ThenTheRefusalStatesTheShortfall asserts the caller is told what they have
// and what they asked for, which is what lets them explain it to their user.
func (l *Ledger) ThenTheRefusalStatesTheShortfall(available, requested Money) error {
	if l.lastAnswer.Available != available {
		return fmt.Errorf("expected %s available, the refusal stated %s", available, l.lastAnswer.Available)
	}
	if l.lastAnswer.Requested != requested {
		return fmt.Errorf("expected %s requested, the refusal stated %s", requested, l.lastAnswer.Requested)
	}
	return nil
}

// ThenTheRepeatIsAnsweredAsAReplay asserts the retry was recognised as one.
func (l *Ledger) ThenTheRepeatIsAnsweredAsAReplay() error {
	if l.lastAnswer.Outcome == Replayed {
		return nil
	}
	return fmt.Errorf("expected the repeat to be answered as a replay, it was %s (answer: %s)",
		l.lastAnswer.Outcome, l.lastAnswer.Raw)
}

// ThenBothAnswersNameTheSameTransaction asserts the retry did not create a
// second transaction.
func (l *Ledger) ThenBothAnswersNameTheSameTransaction() error {
	if l.priorAnswer.TransactionID == "" {
		return fmt.Errorf("the first answer named no transaction, so there is nothing to compare")
	}
	if l.priorAnswer.TransactionID == l.lastAnswer.TransactionID {
		return nil
	}
	return fmt.Errorf("the first answer named %q, the repeat named %q",
		l.priorAnswer.TransactionID, l.lastAnswer.TransactionID)
}

// ThenBothAnswersAreIdentical asserts the caller cannot tell the two apart by
// content — the reason retrying is safe to do blind.
func (l *Ledger) ThenBothAnswersAreIdentical() error {
	if l.priorAnswer.Raw == l.lastAnswer.Raw {
		return nil
	}
	return fmt.Errorf("the answers differ:\n  first:  %s\n  repeat: %s", l.priorAnswer.Raw, l.lastAnswer.Raw)
}

// ThenTheReplayedLegsMatchTheRecordedEntries asserts the replay was rebuilt
// from what is recorded rather than served from a remembered reply (DDD-8). A
// cached body would pass every other replay assertion and fail this one.
func (l *Ledger) ThenTheReplayedLegsMatchTheRecordedEntries(ctx context.Context) error {
	for _, leg := range l.lastAnswer.Legs {
		rows, err := l.readTrace(ctx, leg.Account)
		if err != nil {
			return err
		}
		found := false
		for _, row := range rows {
			if row.TransactionID == l.lastAnswer.TransactionID && row.Amount == leg.Amount {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("the replay reports a leg of %s on %q that is not recorded against transaction %q",
				leg.Amount, leg.Account, l.lastAnswer.TransactionID)
		}
	}
	return nil
}

// --- what the answer said about the movement ------------------------------

// ThenTheAnswerNamesOneTransactionWithLegs asserts the two-leg model is
// discoverable in the answer, which is what makes the ledger explainable later
// (journey post-a-transfer, S3).
func (l *Ledger) ThenTheAnswerNamesOneTransactionWithLegs(first, second Money) error {
	if len(l.lastAnswer.Legs) != 2 {
		return fmt.Errorf("expected 2 legs, the answer carried %d (answer: %s)",
			len(l.lastAnswer.Legs), l.lastAnswer.Raw)
	}
	got := []Money{l.lastAnswer.Legs[0].Amount, l.lastAnswer.Legs[1].Amount}
	want := []Money{first, second}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if got[0] != want[0] || got[1] != want[1] {
		return fmt.Errorf("expected legs %s and %s, got %s and %s", first, second, got[0], got[1])
	}
	if l.lastAnswer.TransactionID == "" {
		return fmt.Errorf("the answer named no transaction")
	}
	return nil
}

// ThenTheAnswerNamesTheTransaction asserts the identifier came from the
// injected generator rather than the store.
func (l *Ledger) ThenTheAnswerNamesTheTransaction(id string) error {
	if l.lastAnswer.TransactionID == id {
		return nil
	}
	return fmt.Errorf("expected transaction %q, the answer named %q", id, l.lastAnswer.TransactionID)
}

// ThenBothLegsBelongToTheSameTransaction asserts atomicity is visible in the
// answer: two legs, one transaction.
func (l *Ledger) ThenBothLegsBelongToTheSameTransaction(ctx context.Context) error {
	for _, leg := range l.lastAnswer.Legs {
		rows, err := l.readTrace(ctx, leg.Account)
		if err != nil {
			return err
		}
		matched := false
		for _, row := range rows {
			if row.TransactionID == l.lastAnswer.TransactionID {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("the leg on %q is not recorded under transaction %q",
				leg.Account, l.lastAnswer.TransactionID)
		}
	}
	return nil
}

// ThenTheTwoLegsSumToZero asserts I1 on the answer itself.
func (l *Ledger) ThenTheTwoLegsSumToZero() error {
	total := Money(0)
	for _, leg := range l.lastAnswer.Legs {
		total += leg.Amount
	}
	if total == 0 {
		return nil
	}
	return fmt.Errorf("the legs of the answer sum to %s, not zero", total)
}

// --- what the ledger holds ------------------------------------------------

// ThenTheBalanceReads asserts an account's balance through the driving port.
func (l *Ledger) ThenTheBalanceReads(ctx context.Context, account AccountName, want Money) error {
	got, err := l.readBalance(ctx, account)
	if err != nil {
		return err
	}
	if got == want {
		return nil
	}
	return fmt.Errorf("expected the balance of %q to read %s, it reads %s", account, want, got)
}

// ThenTheBalanceReadsEither asserts a balance is one of two acceptable values.
// The chaos scenario needs this: an interrupted write may have committed or
// not, and both are correct — a half-applied movement is the only failure.
func (l *Ledger) ThenTheBalanceReadsEither(ctx context.Context, account AccountName, first, second Money) error {
	got, err := l.readBalance(ctx, account)
	if err != nil {
		return err
	}
	if got == first || got == second {
		return nil
	}
	return fmt.Errorf("expected the balance of %q to read %s or %s, it reads %s — a half-applied movement",
		account, first, second, got)
}

// ThenTheLedgerHoldsEntriesSummingToZero asserts both the count and I1 across
// the whole ledger.
func (l *Ledger) ThenTheLedgerHoldsEntriesSummingToZero(ctx context.Context, want int) error {
	report, err := l.readBooks(ctx, HealthSurface)
	if err != nil {
		return err
	}
	if report.EntryCount != want {
		return fmt.Errorf("expected %d entries, the ledger holds %d", want, report.EntryCount)
	}
	if report.TrialBalance != 0 {
		return fmt.Errorf("the entries sum to %s, not zero", report.TrialBalance)
	}
	return nil
}

// ThenEveryBalanceEqualsItsEntries asserts I3 across every account — the
// cross-check slice 04 exists to perform.
func (l *Ledger) ThenEveryBalanceEqualsItsEntries(ctx context.Context) error {
	report, err := l.readBooks(ctx, HealthSurface)
	if err != nil {
		return err
	}
	if len(report.Drifted) == 0 {
		return nil
	}
	var drifted []string
	for _, row := range report.Drifted {
		drifted = append(drifted, fmt.Sprintf("%s (stored %s, computed %s)", row.Account, row.Stored, row.Computed))
	}
	return fmt.Errorf("these accounts disagree with their entries: %s", strings.Join(drifted, ", "))
}

// ThenBothLegsOrNeitherSurvived asserts the atomicity guarantee after an
// interrupted write: the transaction is wholly present or wholly absent.
func (l *Ledger) ThenBothLegsOrNeitherSurvived(ctx context.Context) error {
	report, err := l.readBooks(ctx, HealthSurface)
	if err != nil {
		return err
	}
	if report.EntryCount%2 != 0 {
		return fmt.Errorf("the ledger holds %d entries — an odd count means a movement was half applied", report.EntryCount)
	}
	if report.TrialBalance != 0 {
		return fmt.Errorf("the surviving entries sum to %s — a leg landed without its partner", report.TrialBalance)
	}
	return nil
}

// ThenTheAccountIsOfKind asserts the taxonomy is observable, so "system
// accounts may go negative" is a stated property rather than an accident.
func (l *Ledger) ThenTheAccountIsOfKind(ctx context.Context, account AccountName, kind AccountKind) error {
	if l.accountKind[account] == kind {
		return nil
	}
	return fmt.Errorf("expected %q to be a %s account, the ledger reports %s",
		account, kind, l.accountKind[account])
}

// ThenTheLedgerIsReadyToAcceptATransfer asserts a migrated-from-zero schema is
// actually usable, not merely present.
func (l *Ledger) ThenTheLedgerIsReadyToAcceptATransfer(ctx context.Context) error {
	_, err := l.readBooks(ctx, HealthSurface)
	return err
}

// --- the operator's verdict -----------------------------------------------

// ThenTheVerdictReads asserts the answer to the operator's one question.
func (l *Ledger) ThenTheVerdictReads(want Verdict) error {
	if l.lastReport.Verdict == want {
		return nil
	}
	return fmt.Errorf("expected the verdict to read %q, it reads %q", want, l.lastReport.Verdict)
}

// ThenTheVerdictPrecedesTheFigures asserts the operator is not made to compute
// the verdict from a table (journey verify-the-books, S2).
func (l *Ledger) ThenTheVerdictPrecedesTheFigures() error {
	if l.lastReport.VerdictStatedAt < l.lastReport.FirstFigureStated {
		return nil
	}
	return fmt.Errorf("figures appear at position %d, before the verdict at position %d — the operator has to compute the answer",
		l.lastReport.FirstFigureStated, l.lastReport.VerdictStatedAt)
}

// ThenTheTrialBalanceIs asserts the sum of every entry in the ledger.
func (l *Ledger) ThenTheTrialBalanceIs(want Money) error {
	if l.lastReport.TrialBalance == want {
		return nil
	}
	return fmt.Errorf("expected a trial balance of %s, the verdict reports %s", want, l.lastReport.TrialBalance)
}

// ThenTheVerdictReportsEntriesScanned asserts the denominator of the scan, so
// a verdict computed over nothing cannot report YES.
func (l *Ledger) ThenTheVerdictReportsEntriesScanned(want int) error {
	if l.lastReport.EntryCount == want {
		return nil
	}
	return fmt.Errorf("expected the verdict to report %d entries scanned, it reports %d", want, l.lastReport.EntryCount)
}

// ThenTheVerdictReportsHowLongItTook asserts the D9 degradation signal exists.
func (l *Ledger) ThenTheVerdictReportsHowLongItTook() error {
	if l.lastReport.ElapsedMillis > 0 {
		return nil
	}
	return fmt.Errorf("the verdict reports no scan duration, so D9's deferred checkpointing has no trigger to watch")
}

// ThenBothSurfacesAgree asserts the console is not the only path to the answer.
func (l *Ledger) ThenBothSurfacesAgree() error {
	if l.lastReport.Verdict != l.otherReport.Verdict {
		return fmt.Errorf("the two surfaces disagree: %q and %q", l.otherReport.Verdict, l.lastReport.Verdict)
	}
	return nil
}

// ThenBothSurfacesReportTheSameTrialBalance asserts the figures agree too, not
// just the headline.
func (l *Ledger) ThenBothSurfacesReportTheSameTrialBalance() error {
	if l.lastReport.TrialBalance != l.otherReport.TrialBalance {
		return fmt.Errorf("the two surfaces report %s and %s", l.otherReport.TrialBalance, l.lastReport.TrialBalance)
	}
	return nil
}

// ThenTheAccountIsListedAsDrifted asserts attribution, not merely detection.
func (l *Ledger) ThenTheAccountIsListedAsDrifted(account AccountName, listed bool) error {
	found := false
	for _, row := range l.lastReport.Drifted {
		if row.Account == account {
			found = true
		}
	}
	if found == listed {
		return nil
	}
	if listed {
		return fmt.Errorf("expected %q in the drift listing, it is not there", account)
	}
	return fmt.Errorf("%q is listed as drifted but its entries agree with its balance", account)
}

// ThenExactlyAccountsAreDrifted asserts the blast radius of a single injection.
func (l *Ledger) ThenExactlyAccountsAreDrifted(want int) error {
	if len(l.lastReport.Drifted) == want {
		return nil
	}
	return fmt.Errorf("expected %d drifted account(s), the verdict lists %d", want, len(l.lastReport.Drifted))
}

// ThenTheDriftEntryStatesTheDelta asserts the operator is given stored,
// computed, and delta — a bare NO would pass a detection-only check and still
// fail them (KPI-4).
func (l *Ledger) ThenTheDriftEntryStatesTheDelta(want Money) error {
	for _, row := range l.lastReport.Drifted {
		if row.Delta != want && row.Delta != -want {
			continue
		}
		if row.Stored == row.Computed {
			return fmt.Errorf("the drift row for %q reports the same stored and computed balance, so it explains nothing", row.Account)
		}
		if row.Stored-row.Computed != row.Delta && row.Computed-row.Stored != row.Delta {
			return fmt.Errorf("the drift row for %q reports a delta of %s that does not reconcile stored %s with computed %s",
				row.Account, row.Delta, row.Stored, row.Computed)
		}
		return nil
	}
	return fmt.Errorf("no drift row reports a delta of %s", want)
}

// --- tracing ---------------------------------------------------------------

// ThenTheEntriesAreOldestFirst asserts the history reads in the order it
// happened.
func (l *Ledger) ThenTheEntriesAreOldestFirst() error {
	for i := 1; i < len(l.lastTrace); i++ {
		if l.lastTrace[i-1].RecordedAt > l.lastTrace[i].RecordedAt {
			return fmt.Errorf("row %d is stamped %s, before row %d at %s",
				i, l.lastTrace[i].RecordedAt, i-1, l.lastTrace[i-1].RecordedAt)
		}
	}
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("the trace is empty, so its ordering proves nothing")
	}
	return nil
}

// ThenTheSecondTraceMatchesTheFirst asserts the order is settled, not merely
// sorted once — including for entries sharing a clock tick.
func (l *Ledger) ThenTheSecondTraceMatchesTheFirst(ctx context.Context, account AccountName) error {
	first := l.lastTrace
	if err := l.Trace(ctx, account); err != nil {
		return err
	}
	if len(first) != len(l.lastTrace) {
		return fmt.Errorf("the two traces hold %d and %d rows", len(first), len(l.lastTrace))
	}
	for i := range first {
		if first[i] != l.lastTrace[i] {
			return fmt.Errorf("row %d differs between the two traces: %+v then %+v", i, first[i], l.lastTrace[i])
		}
	}
	return nil
}

// ThenEachRowCarriesARunningBalance asserts the column that makes a break
// point visible actually exists and accumulates.
func (l *Ledger) ThenEachRowCarriesARunningBalance() error {
	running := Money(0)
	for i, row := range l.lastTrace {
		running += row.Amount
		if row.RunningBalance != running {
			return fmt.Errorf("row %d reports a running balance of %s; accumulating the amounts gives %s",
				i, row.RunningBalance, running)
		}
	}
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("the trace is empty, so it carries no running balance")
	}
	return nil
}

// ThenTheFinalRunningBalanceReadsTheStoredBalance asserts the trace reconciles
// on a healthy account.
func (l *Ledger) ThenTheFinalRunningBalanceReadsTheStoredBalance(ctx context.Context, account AccountName) error {
	stored, err := l.readBalance(ctx, account)
	if err != nil {
		return err
	}
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("the trace of %q is empty", account)
	}
	final := l.lastTrace[len(l.lastTrace)-1].RunningBalance
	if final == stored {
		return nil
	}
	return fmt.Errorf("the running balance ends at %s but %q stores %s", final, account, stored)
}

// ThenTheFinalRunningBalanceDisagreesBy asserts the trace makes drift visible
// rather than hiding it.
func (l *Ledger) ThenTheFinalRunningBalanceDisagreesBy(ctx context.Context, account AccountName, want Money) error {
	stored, err := l.readBalance(ctx, account)
	if err != nil {
		return err
	}
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("the trace of %q is empty", account)
	}
	final := l.lastTrace[len(l.lastTrace)-1].RunningBalance
	gap := stored - final
	if gap == want || gap == -want {
		return nil
	}
	return fmt.Errorf("expected the running balance to part company by %s, the gap is %s", want, gap)
}

// ThenTheDivergingRowIsTheAlteredOne asserts the trace points at the guilty
// row, which is the whole diagnostic value of slice 05.
func (l *Ledger) ThenTheDivergingRowIsTheAlteredOne() error {
	running := Money(0)
	for i, row := range l.lastTrace {
		running += row.Amount
		if row.RunningBalance != running {
			if i+1 == l.tamperedRow {
				return nil
			}
			return fmt.Errorf("the running balance first parts company at row %d; the altered entry is row %d", i+1, l.tamperedRow)
		}
	}
	return fmt.Errorf("the running balance never parts company, so the altered entry is invisible in the trace")
}

// ThenEveryRowIsLegible asserts each row carries the four facts that let an
// operator read it without cross-referencing anything by hand.
func (l *Ledger) ThenEveryRowIsLegible() error {
	for i, row := range l.lastTrace {
		switch {
		case row.TransactionID == "":
			return fmt.Errorf("row %d names no transaction", i)
		case row.Counterparty == "":
			return fmt.Errorf("row %d names no counterparty", i)
		case row.RecordedAt == "":
			return fmt.Errorf("row %d says nothing about when it happened", i)
		case row.Amount == 0:
			return fmt.Errorf("row %d moves nothing", i)
		}
	}
	return nil
}

// ThenNoRowIsItsOwnCounterparty asserts the counterparty is the other side of
// the movement, not the account being traced.
func (l *Ledger) ThenNoRowIsItsOwnCounterparty(account AccountName) error {
	for i, row := range l.lastTrace {
		if row.Counterparty == account {
			return fmt.Errorf("row %d names %q as its own counterparty", i, account)
		}
	}
	return nil
}

// ThenExactlyEntriesAreReturned asserts the size of the returned history. Zero
// is the case worth having: an account nothing has happened to must trace to an
// empty listing rather than to an error or to somebody else's rows.
func (l *Ledger) ThenExactlyEntriesAreReturned(want int) error {
	if len(l.lastTrace) == want {
		return nil
	}
	return fmt.Errorf("expected %d entries in the trace, got %d", want, len(l.lastTrace))
}

// ThenTheEntriesOfAreReturned asserts the operator arrived where they meant to.
func (l *Ledger) ThenTheEntriesOfAreReturned(account AccountName) error {
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("no entries were returned for %q", account)
	}
	return nil
}

// ThenEveryEntryIsStamped asserts timestamps come from the injected clock.
func (l *Ledger) ThenEveryEntryIsStamped(stamp string) error {
	for i, row := range l.lastTrace {
		if row.RecordedAt != stamp {
			return fmt.Errorf("row %d is stamped %s, not %s — the clock was read somewhere other than the shell",
				i, row.RecordedAt, stamp)
		}
	}
	if len(l.lastTrace) == 0 {
		return fmt.Errorf("no entries to check the stamp on")
	}
	return nil
}

// --- contended runs and the store's own refusals --------------------------

// ThenExactlyWereAccepted asserts how many of a contended batch posted.
func (l *Ledger) ThenExactlyWereAccepted(want int) error {
	if l.lastRace.Accepted == want {
		return nil
	}
	return fmt.Errorf("expected %d accepted, %d were (of %d attempts)", want, l.lastRace.Accepted, l.lastRace.Attempts)
}

// ThenExactlyWereRefusedFor asserts how many of a contended batch were refused.
func (l *Ledger) ThenExactlyWereRefusedFor(want int, kind RefusalKind) error {
	if kind != InsufficientFunds {
		return fmt.Errorf("contended runs only tally %s refusals", InsufficientFunds)
	}
	if l.lastRace.RefusedInsufficientFunds == want {
		return nil
	}
	return fmt.Errorf("expected %d refused for %s, %d were (of %d attempts)",
		want, kind, l.lastRace.RefusedInsufficientFunds, l.lastRace.Attempts)
}

// ThenNoNegativeBalanceWasObserved asserts I4 held throughout, not just at the
// end — a run that dips negative and recovers has still broken the promise.
func (l *Ledger) ThenNoNegativeBalanceWasObserved() error {
	if l.lastRace.NegativeBalanceObservations == 0 {
		return nil
	}
	return fmt.Errorf("a wallet balance was observed below zero %d time(s)", l.lastRace.NegativeBalanceObservations)
}

// ThenAtLeastAttemptsWereMade asserts the denominator. Without it a harness
// that quietly ran ten iterations reports zero negatives and passes.
func (l *Ledger) ThenAtLeastAttemptsWereMade(want int) error {
	if l.lastRace.Attempts >= want {
		return nil
	}
	return fmt.Errorf("expected at least %d attempts, the harness made %d — the result means nothing at this sample size",
		want, l.lastRace.Attempts)
}

// ThenExactlyTransactionsWereRecordedForTheKey asserts I7 under concurrency.
func (l *Ledger) ThenExactlyTransactionsWereRecordedForTheKey(want int) error {
	if l.lastRace.DistinctTransactionIDs == want {
		return nil
	}
	return fmt.Errorf("expected %d distinct transaction(s) for that key, %d were recorded",
		want, l.lastRace.DistinctTransactionIDs)
}

// ThenExactlyEntryPairsWereRecordedForTheKey catches a double write that
// happens to render the same id — which the id check alone would miss.
func (l *Ledger) ThenExactlyEntryPairsWereRecordedForTheKey(want int) error {
	if l.lastRace.EntryPairsStored == want {
		return nil
	}
	return fmt.Errorf("expected %d entry pair(s) for that key, %d were stored", want, l.lastRace.EntryPairsStored)
}

// ThenAllAnswersNameTheSameTransaction asserts every racing caller was told the
// same thing.
func (l *Ledger) ThenAllAnswersNameTheSameTransaction(count int) error {
	if len(l.lastAnswers) != count {
		return fmt.Errorf("expected %d answers, collected %d", count, len(l.lastAnswers))
	}
	first := l.lastAnswers[0].TransactionID
	for i, answer := range l.lastAnswers {
		if answer.TransactionID != first {
			return fmt.Errorf("answer %d names %q, answer 0 named %q", i, answer.TransactionID, first)
		}
	}
	return nil
}

// ThenEveryAttemptWasAnswered asserts nothing hung.
func (l *Ledger) ThenEveryAttemptWasAnswered() error {
	if l.lastRace.Answered == l.lastRace.Attempts {
		return nil
	}
	return fmt.Errorf("%d of %d attempts went unanswered", l.lastRace.Attempts-l.lastRace.Answered, l.lastRace.Attempts)
}

// ThenNoAttemptDeadlocked asserts the lock ordering rule (DDD-6) held.
func (l *Ledger) ThenNoAttemptDeadlocked() error {
	if l.lastRace.Deadlocked == 0 {
		return nil
	}
	return fmt.Errorf("%d attempt(s) deadlocked — locks were not taken in a settled order", l.lastRace.Deadlocked)
}

// ThenTheAttemptIsRefusedByTheStore asserts D7 is enforced below the
// application, where discipline cannot reach.
func (l *Ledger) ThenTheAttemptIsRefusedByTheStore() error {
	if l.lastTamper != nil {
		return nil
	}
	return fmt.Errorf("recorded history was rewritten — the store permitted it, so D7 is application discipline wearing a database costume")
}

// ThenNoMigrationErasesAnEntry asserts the expand-only rule holds across the
// whole migration set, not just the newest one.
func (l *Ledger) ThenNoMigrationErasesAnEntry(ctx context.Context) error {
	statements, err := postgresMigrationStatements()
	if err != nil {
		return err
	}
	for name, sql := range statements {
		upper := strings.ToUpper(sql)
		if strings.Contains(upper, "DELETE FROM ENTRIES") || strings.Contains(upper, "DROP TABLE ENTRIES") {
			return fmt.Errorf("migration %s erases entries, which D7 forbids", name)
		}
	}
	return nil
}
