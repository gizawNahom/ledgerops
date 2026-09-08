// transfer_coordinator_retry_settlement_test.go closes a gap the two
// existing step 03-02 unit test files leave open: neither
// transfer_coordinator_internal_test.go (backoffForAttempt's own pure
// schedule/jitter math) nor transfer_coordinator_claim_race_test.go (the
// claim gate, observed immediately after the losing goroutine's own failed
// attempt) ever lets scheduleRetry's own detached goroutine actually wake up
// and re-attempt. This test does: it waits out a real backoff window and
// proves the self-rescheduled retry both fires and completes the sequence
// through Leg 3, settling the transfer -- the one behavior
// docs/feature/inter-tenant-transfer/slices/slice-03-retry-a-stalled-leg.md's
// own acceptance criteria describe ("An injected Leg 2 failure results in
// status: retrying, then status: settled once the retry succeeds") that no
// other test in this package currently exercises end-to-end.
//
// WHY-NEW-FILE: internal/app/transfer_coordinator_retry_settlement_test.go
//
//	CLOSEST-EXISTING: transfer_coordinator_claim_race_test.go
//	EXTENSION-COST: the claim race test's own goroutines are torn down
//	  (barrier cleared) specifically so its own leaked, still-sleeping
//	  scheduleRetry goroutine never gets observed -- folding a real-time
//	  wait into that file would mean either waiting inside every one of its
//	  future cases (slowing an otherwise sub-millisecond suite) or awkwardly
//	  special-casing just this one, when a dedicated file states the
//	  real-time-wait cost up front in its own name and doc comment instead.
//	PARALLEL-RATIONALE: distinct concern (does the scheduled retry actually
//	  run and settle) from the claim race test's own concern (does the
//	  claim gate serialize concurrent attempts) -- sharing only the package's
//	  existing fakeStore/sequentialIDs helpers.
package app_test

import (
	"context"
	"testing"
	"time"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// TestAttemptLeg_FailedLegSelfRetriesAndSettlesAfterRealBackoff proves
// scheduleRetry's own detached goroutine is not just scheduled but actually
// wakes up after the real (>=800ms, per backoffJitterSpread's own 20% floor)
// backoff delay and re-enters attemptForwardLegsFrom, completing Leg 2 and
// (since Leg 2 is not leg 3) continuing on to Leg 3, settling the transfer.
// Deliberately uses time.Now (not a fixed clock) and a real time.Sleep --
// this is the one test in the package willing to pay real wall-clock cost to
// observe the retry path a fixed clock or a single synchronous call can
// never exercise.
func TestAttemptLeg_FailedLegSelfRetriesAndSettlesAfterRealBackoff(t *testing.T) {
	amount, err := domain.NewMoney(5000, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	zero, err := domain.NewMoney(0, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}

	// Leg 2 moves tnt_acme's own platform mirror to tnt_beacon's own
	// platform mirror, both under tnt_platform (legMovement); Leg 3 moves
	// tnt_beacon's own settlement account to its real target account, under
	// tnt_beacon.
	platformFrom, err := domain.NewAccount("tnt_platform", "platform-tnt_acme", domain.System, amount)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	platformTo, err := domain.NewAccount("tnt_platform", "platform-tnt_beacon", domain.System, zero)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	settlement, err := domain.NewAccount("tnt_beacon", "settlement", domain.System, amount)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	targetWallet, err := domain.NewAccount("tnt_beacon", "beacon-wallet-ops", domain.Wallet, zero)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}

	store := newFakeStore(platformFrom, platformTo, settlement, targetWallet)
	ledger := app.NewLedger(store, time.Now, sequentialIDs())
	tc := app.NewTransferCoordinator(ledger)
	ctx := context.Background()

	state := ports.TransferState{
		TransferID:           "xfr_retry_settlement",
		TenantID:             "tnt_acme",
		IdempotencyKey:       "idem-retry-settlement",
		Status:               "pending",
		Leg1Status:           "posted",
		Leg2Status:           "pending",
		Leg3Status:           "pending",
		NextAttemptAt:        time.Now().Add(-time.Hour),
		CounterpartyTenantID: "tnt_beacon",
		TargetAccountID:      "beacon-wallet-ops",
		Amount:               amount,
	}
	if err := tc.SeedTransferStateDueNow(ctx, state); err != nil {
		t.Fatalf("SeedTransferStateDueNow: %v", err)
	}

	// One-shot fault: only Leg 2's very first attempt fails, so the
	// self-rescheduled retry attemptLeg re-enters through must succeed.
	tc.InjectLegFault(state.TransferID, 2)

	if _, err := tc.ProcessDueTransfersOnce(ctx); err != nil {
		t.Fatalf("ProcessDueTransfersOnce: %v", err)
	}

	view, err := tc.GetTransfer(ctx, state.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if view.Status != "retrying" {
		t.Fatalf("expected \"retrying\" immediately after the injected failure, got %q", view.Status)
	}

	// backoffForAttempt(1, jitter) lands in [0.8s, 1.2s) -- 1.5s comfortably
	// clears the top of that band without padding the suite unnecessarily.
	time.Sleep(1500 * time.Millisecond)

	view, err = tc.GetTransfer(ctx, state.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if view.Status != "settled" {
		t.Fatalf("expected the self-rescheduled retry to settle the transfer, got %q (leg2=%q leg3=%q)",
			view.Status, view.Leg2.Status, view.Leg3.Status)
	}
}
