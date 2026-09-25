// transfer_coordinator_claim_race_test.go is step 03-02's own Earned Trust
// obligation (roadmap DESIGN_CONTEXT): "ClaimOne's probe() must exercise the
// losing side of a claim race and confirm it returns immediately without
// attempting anything" -- specifically at the attemptLeg level, not merely
// the repository level internal/adapters/postgres/transfer_state_test.go's
// own TestTransferStateRepository_ClaimOne_ConcurrentClaimsOnTheSameRow_
// ExactlyOneWins already covers. Two real goroutines race
// TransferCoordinator.ProcessDueTransfersOnce, both discovering the
// identical due transfer_id through ClaimDue, both then converging on
// attemptLeg's own ClaimOne call for the same (transfer_id, leg) pair.
//
// WHY-NEW-FILE: internal/app/transfer_coordinator_claim_race_test.go
//
//	CLOSEST-EXISTING: internal/app/usecases_test.go
//	EXTENSION-COST: usecases_test.go's own suite exercises PostTransfer's
//	  single-unit-of-work orchestration; this test's own subject
//	  (concurrent attemptLeg claim races) needs goroutine synchronization
//	  primitives (start gate + WaitGroup) and its own transfer_state
//	  fixture shape that would be an unrelated addition bolted onto an
//	  already-large existing test function rather than a natural extension
//	  of it.
//	PARALLEL-RATIONALE: distinct concern (concurrency safety of a different
//	  driving-port method, ProcessDueTransfersOnce/attemptLeg, not
//	  PostTransfer) with its own setup shape (transfer_state fixture +
//	  synchronized goroutines) that shares no code with
//	  TestProperty_PostTransfer_MovesValueAndClaimsTheKeyAtomically beyond
//	  the already-shared fakeStore/app.NewLedger helpers.
package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// TestAttemptLeg_ConcurrentClaimRace_ExactlyOneCallerAttemptsThePost proves
// ClaimOne's double-processing guard holds at attemptLeg's own call level
// (not just the repository level): two goroutines racing
// ProcessDueTransfersOnce against the identical due transfer converge on
// the same ClaimOne call inside attemptLeg, and exactly one of them ever
// gets past it — the loser's attemptLeg returns immediately, never
// attempting a Post.
//
// A fault is injected up front so the winner's own attempt fails
// deterministically without needing any real account/balance fixture --
// legMovement itself is a pure decision from the persisted TransferState
// alone, and consumeInjectedLegFault short-circuits before ledger.PostTransfer
// is ever called, so this test needs no accounts at all to prove the claim
// gate itself. store.claimAttempts/claimWins (fakes_test.go) are the direct
// observation: two calls in, exactly one win, mirroring
// TestTransferStateRepository_ClaimOne_ConcurrentClaimsOnTheSameRow_
// ExactlyOneWins's own repository-level probe one layer up.
func TestAttemptLeg_ConcurrentClaimRace_ExactlyOneCallerAttemptsThePost(t *testing.T) {
	store := newFakeStore()
	// time.Now (not fixedClock, unlike the rest of this suite): the fake's
	// own ClaimOne (fakes_test.go) evaluates due-ness against the real wall
	// clock, mirroring the real adapter's own time.Now()-based lease check.
	// handleLegFailure's own next_attempt_at write reads this same
	// coordinator clock -- using fixedClock's fixed 2026-08-20 here would
	// compute a backoff target already in the past relative to ClaimOne's
	// real-clock check, making the second attempt look due immediately
	// instead of genuinely deferred.
	ledger := app.NewLedger(store, time.Now, sequentialIDs())
	tc := app.NewTransferCoordinator(ledger)
	ctx := context.Background()

	amount, err := domain.NewMoney(5000, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	state := ports.TransferState{
		TransferID:     "xfr_claim_race",
		TenantID:       "tnt_acme",
		IdempotencyKey: "idem-claim-race",
		Status:         "pending",
		Leg1Status:     "posted",
		Leg2Status:     "pending",
		Leg3Status:     "pending",
		// Deliberately a fixed, far-past instant, not time.Now(): ClaimDue
		// below is evaluated against the coordinator's own injected
		// fixedClock (2026-08-20, see usecases_test.go), while the fake's
		// ClaimOne (fakes_test.go, mirroring the real adapter's own
		// time.Now()-based lease check) is evaluated against the real wall
		// clock -- a value before both never risks landing "not yet due"
		// against either.
		NextAttemptAt:        time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CounterpartyTenantID: "tnt_beacon",
		TargetAccountID:      "beacon-wallet-ops",
		Amount:               amount,
	}
	if err := tc.SeedTransferStateDueNow(ctx, state); err != nil {
		t.Fatalf("SeedTransferStateDueNow: %v", err)
	}

	// Leg 2's very first attempt (whichever goroutine wins the claim) is
	// armed to fail -- this test's own subject is the claim gate, not the
	// posting outcome, so no account fixture is needed at all (see the
	// function doc comment above).
	tc.InjectLegFault(state.TransferID, 2)

	// claimBarrier forces both racing goroutines to actually be inside
	// ClaimOne together before either proceeds -- see its own doc comment
	// on fakeStore for why an in-memory fake needs this explicit rendezvous
	// where a real SELECT ... FOR UPDATE would get true overlap for free
	// from transaction blocking.
	const racers = 2
	barrier := &sync.WaitGroup{}
	barrier.Add(racers)
	store.claimBarrier = barrier

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
	)
	start.Add(1)
	done.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer done.Done()
			start.Wait() // release both goroutines together, maximizing overlap
			_, _ = tc.ProcessDueTransfersOnce(ctx)
		}()
	}
	start.Done()
	done.Wait()

	store.claimMu.Lock()
	attempts, wins := store.claimAttempts, store.claimWins
	// The winner's own losing attempt (fault-injected) schedules a real,
	// budget-remaining retry ~1s+jitter later (scheduleRetry) -- outliving
	// this test function. Clearing claimBarrier now, under the same lock
	// ClaimOne itself reads it under, means that leaked retry's own later
	// ClaimOne call takes the barrier==nil branch instead of calling Done()
	// on an already-exhausted (and, by then, long-since-out-of-scope)
	// WaitGroup.
	store.claimBarrier = nil
	store.claimMu.Unlock()

	if attempts != racers {
		t.Fatalf("expected both racing goroutines to reach ClaimOne (attempts=%d), got %d", racers, attempts)
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner among %d concurrent ClaimOne calls on the same row, got %d", racers, wins)
	}

	view, err := tc.GetTransfer(ctx, state.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if view.Status != "retrying" {
		t.Fatalf("expected the winner's injected-fault attempt to leave the transfer \"retrying\", got %q", view.Status)
	}
}
