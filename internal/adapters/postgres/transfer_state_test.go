// Adapter integration test for the driven postgres TransferStateRepository,
// real PostgreSQL 16 via Testcontainers (OPS-11, Mandate 6). Most of these
// are wiring tests, not property tests, per the layered test discipline
// (nw-tdd-methodology § Layered test discipline): "Integration | UNCHANGED —
// single-example test verifies WIRING." The concurrent-claim race test
// below is this step's own Earned Trust obligation and is deliberately not
// a single-example wiring test — it drives two real, concurrent Postgres
// connections against the identical row.
//
// This step (02-03) has no pre-authored acceptance test reaching this
// layer — the coordinator itself (attemptLeg, the ticker) lands in later
// steps (02-05/03-02) — so this adapter test IS this step's own RED/GREEN
// obligation, mirroring tenant_links_test.go's own note for step 01-03.
package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"ledgerops/internal/app/ports"
)

// transferStateFixture builds a TransferState ready to Create, with
// next_attempt_at already due (ADR-015 Amendment: never NULL, due from the
// instant of creation). dueAt is truncated to microsecond precision —
// Postgres's timestamptz column stores microseconds, not Go's nanoseconds,
// so an expected value built at full Go precision would never
// byte-compare equal to what a round trip returns.
func transferStateFixture(transferID string, dueAt time.Time) ports.TransferState {
	return ports.TransferState{
		TransferID:     transferID,
		TenantID:       "tnt_acme",
		IdempotencyKey: "idem-" + transferID,
		Status:         "pending",
		Leg1Status:     "posted",
		Leg2Status:     "pending",
		Leg3Status:     "pending",
		NextAttemptAt:  dueAt.Truncate(time.Microsecond),
	}
}

// TestTransferStateRepository_CreateThenGet_RoundTripsThroughRealPostgres
// covers the wiring itself: a row inserted through Create reads back
// byte-identical through Get.
func TestTransferStateRepository_CreateThenGet_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	state := transferStateFixture("xfr_roundtrip", time.Now().UTC().Add(-time.Second))

	writeUOW := beginUOW(t, store)
	if err := writeUOW.TransferStates().Create(ctx, state); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, found, err := readUOW.TransferStates().Get(ctx, "xfr_roundtrip")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatalf("Get() found = false, want true")
	}
	if got.TransferID != state.TransferID || got.TenantID != state.TenantID ||
		got.IdempotencyKey != state.IdempotencyKey || got.Status != state.Status ||
		got.Leg1Status != state.Leg1Status || got.Leg2Status != state.Leg2Status || got.Leg3Status != state.Leg3Status ||
		got.Leg1Attempts != 0 || got.Leg2Attempts != 0 || got.Leg3Attempts != 0 ||
		got.Reason != "" || !got.NextAttemptAt.Equal(state.NextAttemptAt) {
		t.Fatalf("Get() = %+v, want %+v", got, state)
	}
}

// TestTransferStateRepository_Get_AbsentIDIsNotFoundNotError proves Get's
// expected shape of "no": an id never created answers (zero value, false,
// nil), not an error — mirroring TenantLinkRepository.ActiveByPair's own
// contract.
func TestTransferStateRepository_Get_AbsentIDIsNotFoundNotError(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	got, found, err := uow.TransferStates().Get(ctx, "xfr_never-created")
	_ = uow.Rollback(ctx)

	if err != nil {
		t.Fatalf("Get unexpectedly errored: %v", err)
	}
	if found {
		t.Fatalf("Get() found = true, want false for an absent id")
	}
	if got != (ports.TransferState{}) {
		t.Fatalf("Get() = %+v, want zero value", got)
	}
}

// TestTransferStateRepository_UpdateStatus_TransitionsStatusAndReason
// covers the coordinator's own lifecycle write: UpdateStatus changes the
// top-level status and records a terminal-state reason, leaving every
// other stored field untouched.
func TestTransferStateRepository_UpdateStatus_TransitionsStatusAndReason(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	state := transferStateFixture("xfr_update-status", time.Now().UTC().Add(-time.Second))
	writeUOW := beginUOW(t, store)
	if err := writeUOW.TransferStates().Create(ctx, state); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	updateUOW := beginUOW(t, store)
	if err := updateUOW.TransferStates().UpdateStatus(ctx, "xfr_update-status", "reversed", "retry_budget_exhausted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := updateUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (update): %v", err)
	}

	readUOW := beginUOW(t, store)
	got, found, err := readUOW.TransferStates().Get(ctx, "xfr_update-status")
	_ = readUOW.Rollback(ctx)
	if err != nil || !found {
		t.Fatalf("Get = %+v, %v, %v", got, found, err)
	}
	if got.Status != "reversed" {
		t.Fatalf("Status = %q, want %q", got.Status, "reversed")
	}
	if got.Reason != "retry_budget_exhausted" {
		t.Fatalf("Reason = %q, want %q", got.Reason, "retry_budget_exhausted")
	}
	if got.Leg1Status != state.Leg1Status || got.IdempotencyKey != state.IdempotencyKey {
		t.Fatalf("UpdateStatus mutated fields outside its own contract: got %+v", got)
	}
}

// TestTransferStateRepository_UpdateStatus_UnknownIDIsRefused mirrors this
// project's absent-row-is-refused convention (e.g.
// TenantLinkRepository.Revoke): naming a transfer_id nothing ever created
// is refused rather than silently succeeding with zero rows affected.
func TestTransferStateRepository_UpdateStatus_UnknownIDIsRefused(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	err := uow.TransferStates().UpdateStatus(ctx, "xfr_never-created", "reversed", "retry_budget_exhausted")
	_ = uow.Rollback(ctx)

	if err == nil {
		t.Fatalf("UpdateStatus on an unknown transfer unexpectedly succeeded")
	}
}

// TestTransferStateRepository_ClaimOne_ClaimsAnEligibleRowAndExtendsLease
// covers the wiring itself: a row already due (next_attempt_at in the
// past, status pending) is claimed, ok=true, and its next_attempt_at is
// extended by leaseDuration.
func TestTransferStateRepository_ClaimOne_ClaimsAnEligibleRowAndExtendsLease(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	dueAt := time.Now().UTC().Add(-5 * time.Second)
	state := transferStateFixture("xfr_claim-eligible", dueAt)
	seedUOW := beginUOW(t, store)
	if err := seedUOW.TransferStates().Create(ctx, state); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := seedUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}

	claimUOW := beginUOW(t, store)
	claimed, ok, err := claimUOW.TransferStates().ClaimOne(ctx, "xfr_claim-eligible", 15*time.Second)
	if err != nil {
		t.Fatalf("ClaimOne: %v", err)
	}
	if !ok {
		t.Fatalf("ClaimOne() ok = false, want true for an eligible row")
	}
	if !claimed.NextAttemptAt.After(dueAt) {
		t.Fatalf("ClaimOne() did not extend next_attempt_at: got %v, was %v", claimed.NextAttemptAt, dueAt)
	}
	if err := claimUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (claim): %v", err)
	}
}

// TestTransferStateRepository_ClaimOne_RefusesANotYetDueRow proves the
// eligibility half of the predicate: a row whose next_attempt_at is still
// in the future is not claimed, ok=false, no error — the "not eligible"
// outcome ADR-015 Amendment 2 names as normal, not a failure.
func TestTransferStateRepository_ClaimOne_RefusesANotYetDueRow(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	notYetDue := time.Now().UTC().Add(time.Hour)
	state := transferStateFixture("xfr_not-yet-due", notYetDue)
	seedUOW := beginUOW(t, store)
	if err := seedUOW.TransferStates().Create(ctx, state); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := seedUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}

	claimUOW := beginUOW(t, store)
	_, ok, err := claimUOW.TransferStates().ClaimOne(ctx, "xfr_not-yet-due", 15*time.Second)
	_ = claimUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ClaimOne unexpectedly errored: %v", err)
	}
	if ok {
		t.Fatalf("ClaimOne() ok = true, want false for a not-yet-due row")
	}
}

// TestTransferStateRepository_ClaimOne_RefusesAnUnknownID proves the
// absent-row half of the predicate: claiming a transfer_id nothing ever
// created answers ok=false, no error — not an infrastructure error.
func TestTransferStateRepository_ClaimOne_RefusesAnUnknownID(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	_, ok, err := uow.TransferStates().ClaimOne(ctx, "xfr_never-created", 15*time.Second)
	_ = uow.Rollback(ctx)
	if err != nil {
		t.Fatalf("ClaimOne unexpectedly errored: %v", err)
	}
	if ok {
		t.Fatalf("ClaimOne() ok = true, want false for an unknown id")
	}
}

// TestTransferStateRepository_ClaimOne_ConcurrentClaimsOnTheSameRow_ExactlyOneWins
// is this step's Earned Trust obligation: it empirically proves the single
// most safety-critical property in this feature — that two concurrent
// ClaimOne calls on the identical due row can never both win. This is what
// prevents double-processing a leg (ADR-015 Amendment 2). Two goroutines,
// two independent unit-of-work transactions against the same real
// PostgreSQL container, race ClaimOne on one seeded row; exactly one must
// return ok=true, the other ok=false with no error.
func TestTransferStateRepository_ClaimOne_ConcurrentClaimsOnTheSameRow_ExactlyOneWins(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	dueAt := time.Now().UTC().Add(-5 * time.Second)
	state := transferStateFixture("xfr_race", dueAt)
	seedUOW := beginUOW(t, store)
	if err := seedUOW.TransferStates().Create(ctx, state); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := seedUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}

	const attempts = 2
	results := make([]bool, attempts)
	errs := make([]error, attempts)

	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			uow, err := store.Begin(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			_, ok, err := uow.TransferStates().ClaimOne(ctx, "xfr_race", 15*time.Second)
			if err != nil {
				errs[i] = err
				_ = uow.Rollback(ctx)
				return
			}
			results[i] = ok
			if ok {
				errs[i] = uow.Commit(ctx)
			} else {
				errs[i] = uow.Rollback(ctx)
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d: unexpected error %v", i, err)
		}
	}

	wins := 0
	for _, ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner among %d concurrent ClaimOne calls on the same row, got %d (results=%v)", attempts, wins, results)
	}
}

// TestTransferStateRepository_ClaimDue_NeverWrites_AndOrdersOldestFirst is
// this step's explicit read-only proof for ClaimDue (QUALITY_GATES:
// "ClaimDue never writes"): discovering due transfers must not change
// next_attempt_at, status, or any other stored column — a subsequent
// ClaimOne against a discovered id must still succeed exactly as if
// ClaimDue had never run. Also covers oldest-due-first ordering.
func TestTransferStateRepository_ClaimDue_NeverWrites_AndOrdersOldestFirst(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	now := time.Now().UTC()
	older := transferStateFixture("xfr_older", now.Add(-10*time.Second))
	newer := transferStateFixture("xfr_newer", now.Add(-1*time.Second))
	notDueYet := transferStateFixture("xfr_future", now.Add(time.Hour))

	seedUOW := beginUOW(t, store)
	for _, s := range []ports.TransferState{older, newer, notDueYet} {
		if err := seedUOW.TransferStates().Create(ctx, s); err != nil {
			t.Fatalf("Create(%q): %v", s.TransferID, err)
		}
	}
	if err := seedUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}

	discoverUOW := beginUOW(t, store)
	due, err := discoverUOW.TransferStates().ClaimDue(ctx, now, 100)
	_ = discoverUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(due) != 2 || due[0] != "xfr_older" || due[1] != "xfr_newer" {
		t.Fatalf("ClaimDue() = %v, want [xfr_older xfr_newer] (oldest-due-first, future row excluded)", due)
	}

	// Never-writes proof: the discovered older row must still be exactly
	// as due as it was before ClaimDue ran, and ClaimOne against it must
	// still succeed -- if ClaimDue had silently mutated next_attempt_at
	// or status, this claim would now fail.
	verifyUOW := beginUOW(t, store)
	got, found, err := verifyUOW.TransferStates().Get(ctx, "xfr_older")
	_ = verifyUOW.Rollback(ctx)
	if err != nil || !found {
		t.Fatalf("Get(xfr_older) = %+v, %v, %v", got, found, err)
	}
	if got.Status != "pending" || !got.NextAttemptAt.Equal(older.NextAttemptAt) {
		t.Fatalf("ClaimDue mutated xfr_older's stored state: got %+v, want status=pending next_attempt_at=%v", got, older.NextAttemptAt)
	}

	claimUOW := beginUOW(t, store)
	_, ok, err := claimUOW.TransferStates().ClaimOne(ctx, "xfr_older", 15*time.Second)
	if err != nil {
		t.Fatalf("ClaimOne after ClaimDue: %v", err)
	}
	if !ok {
		t.Fatalf("ClaimOne(xfr_older) after ClaimDue ok = false, want true -- ClaimDue must never write")
	}
	if err := claimUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (claim): %v", err)
	}
}

// TestTransferStateRepository_ClaimDue_RespectsBatchLimit is the
// design-doc-named test obligation ("seed more than batchLimit due rows,
// run one processDueTransfers tick, assert exactly batchLimit were
// claimed and the remainder are claimed on the next tick") at this step's
// own layer: ClaimDue itself never claims (it is discovery-only), so here
// that translates to "ClaimDue returns at most batchLimit ids even when
// more are due, oldest-first, and the remainder appear on a second call".
func TestTransferStateRepository_ClaimDue_RespectsBatchLimit(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()
	provisionTenant(t, store, "tnt_acme", "acme")

	now := time.Now().UTC()
	const totalDue = 3
	const batchLimit = 2

	seedUOW := beginUOW(t, store)
	for i := 0; i < totalDue; i++ {
		// Oldest first ("xfr_batch-0") so the batch-limited discovery's
		// first page is deterministic.
		dueAt := now.Add(-time.Duration(totalDue-i) * time.Second)
		id := "xfr_batch-" + string(rune('0'+i))
		if err := seedUOW.TransferStates().Create(ctx, transferStateFixture(id, dueAt)); err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
	}
	if err := seedUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}

	firstUOW := beginUOW(t, store)
	firstPage, err := firstUOW.TransferStates().ClaimDue(ctx, now, batchLimit)
	_ = firstUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ClaimDue (first page): %v", err)
	}
	if len(firstPage) != batchLimit {
		t.Fatalf("ClaimDue() returned %d ids, want batchLimit=%d", len(firstPage), batchLimit)
	}

	// Claim the first page (as processDueTransfers would), so the second
	// ClaimDue call's remainder is unambiguous.
	for _, id := range firstPage {
		claimUOW := beginUOW(t, store)
		_, ok, err := claimUOW.TransferStates().ClaimOne(ctx, id, 15*time.Second)
		if err != nil || !ok {
			t.Fatalf("ClaimOne(%q) = %v, %v, want ok=true", id, ok, err)
		}
		if err := claimUOW.Commit(ctx); err != nil {
			t.Fatalf("Commit (claim %q): %v", id, err)
		}
	}

	secondUOW := beginUOW(t, store)
	secondPage, err := secondUOW.TransferStates().ClaimDue(ctx, now, batchLimit)
	_ = secondUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ClaimDue (second page): %v", err)
	}
	if len(secondPage) != totalDue-batchLimit {
		t.Fatalf("ClaimDue() (second page) = %v, want exactly the %d remaining ids", secondPage, totalDue-batchLimit)
	}
}
