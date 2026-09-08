// transfer_coordinator_internal_test.go is a white-box (package app) unit
// test file — an exception to this package's own app_test convention
// (usecases_test.go's own doc comment), justified because backoffForAttempt
// is deliberately unexported: it is a pure, internal implementation detail
// of attemptLeg's own retry mechanics (step 03-02), not part of any driving
// port this package exposes. No paired AT or step definition reaches this
// function directly, so it has no other place to be verified from.
//
// WHY-NEW-FILE: internal/app/transfer_coordinator_internal_test.go
//   CLOSEST-EXISTING: internal/app/usecases_test.go
//   EXTENSION-COST: usecases_test.go is package app_test (black-box, driving-
//     port only) — adding a white-box case there would require either
//     exporting backoffForAttempt (leaking an implementation detail past the
//     port boundary) or splitting the file's own package declaration, which
//     would silently flip every other test in it to white-box scope too.
//   PARALLEL-RATIONALE: a distinct package declaration (app, not app_test)
//     is an incompatible compilation unit boundary Go itself enforces, not a
//     stylistic preference — the two cannot share one file.
package app

import (
	"testing"
	"time"
)

// TestBackoffForAttempt_MatchesTheDocumentedScheduleWithinJitterBand proves
// the exact 1s/2s/4s/8s + ~20% jitter schedule (wave-decisions.md § Key
// Decisions: "Fixed retry budget N = 5, exponential backoff 1s/2s/4s/8s +
// ~20% jitter") — every failedAttempts value the budget allows (1..4), at
// the jitter band's own two extremes (0.0 and the highest representable
// value below 1.0), lands within [0.8x, 1.2x) of its base delay.
func TestBackoffForAttempt_MatchesTheDocumentedScheduleWithinJitterBand(t *testing.T) {
	cases := []struct {
		failedAttempts int
		base           time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
	}

	for _, tc := range cases {
		for _, jitter := range []float64{0, 0.25, 0.5, 0.75, 0.999999} {
			delay, ok := backoffForAttempt(tc.failedAttempts, jitter)
			if !ok {
				t.Fatalf("backoffForAttempt(%d, %v) ok = false, want true (budget not yet exhausted)", tc.failedAttempts, jitter)
			}
			lower := time.Duration(float64(tc.base) * 0.8)
			upper := time.Duration(float64(tc.base) * 1.2)
			if delay < lower || delay >= upper {
				t.Fatalf("backoffForAttempt(%d, %v) = %v, want within [%v, %v)", tc.failedAttempts, jitter, delay, lower, upper)
			}
		}
	}
}

// TestBackoffForAttempt_ExhaustsAtTheFixedBudgetOfFive proves N=5 is a hard
// stop: attempt 5 (the fifth failure) and beyond report the budget
// exhausted, never scheduling a further delay.
func TestBackoffForAttempt_ExhaustsAtTheFixedBudgetOfFive(t *testing.T) {
	for _, failedAttempts := range []int{5, 6, 100} {
		if _, ok := backoffForAttempt(failedAttempts, 0.5); ok {
			t.Fatalf("backoffForAttempt(%d, 0.5) ok = true, want false (budget of 5 exhausted)", failedAttempts)
		}
	}
}

// TestBackoffForAttempt_RejectsAZeroOrNegativeAttemptCount proves the
// function's own lower bound: failedAttempts must be at least 1 (there is
// no "0th failure" to schedule a retry after).
func TestBackoffForAttempt_RejectsAZeroOrNegativeAttemptCount(t *testing.T) {
	for _, failedAttempts := range []int{0, -1} {
		if _, ok := backoffForAttempt(failedAttempts, 0.5); ok {
			t.Fatalf("backoffForAttempt(%d, 0.5) ok = true, want false", failedAttempts)
		}
	}
}
