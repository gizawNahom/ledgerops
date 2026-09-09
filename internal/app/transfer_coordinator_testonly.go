// transfer_coordinator_testonly.go isolates the test-only fault-injection
// seam (step 03-01) TransferCoordinator carries — extracted (deliver-level
// Phase 3 refactor) from transfer_coordinator.go so a reader of the saga's
// own file does not have to wade through the fault-injection/crash-
// simulation surface to follow the forward/reversal leg-attempt logic, and
// vice versa. Same package (app), same unexported types/fields, same
// receiver (*TransferCoordinator) — this is pure code motion, not a new
// abstraction: every call site in transfer_coordinator.go is unchanged.
//
// Every method below exists for exactly one reason: the acceptance suite
// needs a named back door to force a specific leg to fail, or to simulate a
// process crash at one of the two crash windows, or to seed rows a ticker
// tick can discover — deterministically, without real network faults or
// real wall-clock waiting. None of these methods is reachable from any
// production driving port; only tests/acceptance/intertenanttransfer/world.go,
// through a testonly-build-tagged HTTP adapter
// (internal/adapters/http/testonly_faults.go), ever calls them.
package app

import (
	"errors"
	"sync"
)

// testOnlyFaultState is a per-coordinator (not global) registry, so two
// World instances in the same test binary never share fault state across
// scenarios (each scenario's own httptest.Server wires a fresh
// TransferCoordinator via NewRouter).
type testOnlyFaultState struct {
	mu        sync.Mutex
	legFaults map[string]int // key: transferID + ":" + leg, remaining fail count
	// (2026-09-08, DELIVER 03-04 back-propagation): widened from
	// map[string]bool to map[string]int so InjectLegFaultCount can arm a
	// SPECIFIC number of consecutive failures ("fails on its first two
	// attempts"), not just a single one-shot fault -- InjectLegFault's own
	// one-shot behavior is unchanged, expressed as count=1 below.
	skipForwardCrash  map[string]bool // key: transferID
	skipReversalCrash map[string]bool // key: transferID — recorded for 03-02/04's own reversal path to consult
}

// InjectLegFault forces the next attemptLeg call for the named transfer's
// leg to fail with a simulated transient fault, consumed on first use — the
// following attempt (inline retry or ticker) runs normally. Equivalent to
// InjectLegFaultCount(transferID, leg, 1).
func (tc *TransferCoordinator) InjectLegFault(transferID string, leg int) {
	tc.InjectLegFaultCount(transferID, leg, 1)
}

// InjectLegFaultCount forces the next `count` consecutive attemptLeg calls
// for the named transfer's leg to each fail with a simulated transient
// fault (2026-09-08, DELIVER 03-04 back-propagation) — added for "fails on
// its first two attempts," which InjectLegFault's fixed one-shot semantics
// could not express. count <= 0 arms nothing (a no-op), mirroring
// InjectLegFault's own always-arm-exactly-one contract by construction for
// count == 1.
func (tc *TransferCoordinator) InjectLegFaultCount(transferID string, leg, count int) {
	tc.armFault(transferLegKey(transferID, leg), count)
}

// InjectReversalFaultCount forces the next `count` consecutive compensating-
// reversal Post attempts for the named transfer's leg-N reversal (leg 1 or
// 2) to each fail with a simulated transient fault (step 04-03) — the
// reversal-side mirror of InjectLegFaultCount above, needed because a
// reversal Post can itself fail repeatedly and exhaust its own retry
// budget (Amendment 3). Keyed under the disjoint transferReversalLegKey
// namespace, so arming a leg-1 reversal fault can never be mistaken for
// arming a forward leg-1 fault (leg 1 has no forward retry budget of its
// own to begin with — see this file's own SendTransfer doc comment).
func (tc *TransferCoordinator) InjectReversalFaultCount(transferID string, leg, count int) {
	tc.armFault(transferReversalLegKey(transferID, leg), count)
}

// armFault is the shared arm-a-fault-count mechanism InjectLegFaultCount and
// InjectReversalFaultCount both reduce to — one map, one locking discipline,
// keyed by whichever namespace (forward vs. reversal) the caller supplies.
func (tc *TransferCoordinator) armFault(key string, count int) {
	if count <= 0 {
		return
	}
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.legFaults[key] = count
}

// consumeInjectedLegFault reports whether a fault was armed for this
// transfer's leg, decrementing the remaining count and clearing the entry
// once exhausted — a fault only ever stalls the next `count` attempts it
// meets, never every attempt after it.
func (tc *TransferCoordinator) consumeInjectedLegFault(transferID string, leg int) bool {
	return tc.consumeFault(transferLegKey(transferID, leg))
}

// consumeInjectedReversalFault is consumeInjectedLegFault's own reversal-side
// mirror (step 04-03) — checked from postLeg1Reversal/postLeg2Reversal, the
// two Post-issuing functions a reversal attempt's own retry loop calls.
func (tc *TransferCoordinator) consumeInjectedReversalFault(transferID string, leg int) bool {
	return tc.consumeFault(transferReversalLegKey(transferID, leg))
}

// consumeFault is armFault's own consume-side counterpart, shared by both
// consumeInjectedLegFault and consumeInjectedReversalFault.
func (tc *TransferCoordinator) consumeFault(key string) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	remaining := tc.testOnly.legFaults[key]
	if remaining <= 0 {
		return false
	}
	remaining--
	if remaining <= 0 {
		delete(tc.testOnly.legFaults, key)
	} else {
		tc.testOnly.legFaults[key] = remaining
	}
	return true
}

// errSimulatedTransientFault is the injected failure attemptLeg's own
// recordLegOutcome sees — indistinguishable, from that call's own
// perspective, from a genuine transient Post failure.
var errSimulatedTransientFault = errors.New("simulated transient fault (test-only fault injection)")

// SimulateCrashBeforeForwardLegAttempt marks a transfer so spawnForwardLegs'
// own goroutine returns immediately without ever attempting leg 2 —
// simulating a process crash between Leg 1's commit and the inline
// goroutine's first Leg 2 attempt. The retry ticker (processDueTransfers)
// remains the only path that can ever claim the row afterward.
func (tc *TransferCoordinator) SimulateCrashBeforeForwardLegAttempt(transferID string) {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.skipForwardCrash[transferID] = true
}

// consumeForwardCrashSimulation reports (and clears) whether this transfer
// was marked to skip its inline forward attempt entirely.
func (tc *TransferCoordinator) consumeForwardCrashSimulation(transferID string) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	if tc.testOnly.skipForwardCrash[transferID] {
		delete(tc.testOnly.skipForwardCrash, transferID)
		return true
	}
	return false
}

// consumeReversalCrashSimulation reports (and clears) whether this transfer
// was marked to skip its inline leg-1-reversal attempt (04-02) -- the
// compensating-side mirror of consumeForwardCrashSimulation above. Checked
// ONLY from the inline reversal path (reverseLeg2ThenLeg1, isInlinePath
// true): the ticker's own resumeLeg1Reversal path never consults this flag,
// exactly as attemptLeg's own isInlinePath gate keeps the ticker from ever
// mistaking a forward-crash flag for its own instruction to stand down.
func (tc *TransferCoordinator) consumeReversalCrashSimulation(transferID string) bool {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	if tc.testOnly.skipReversalCrash[transferID] {
		delete(tc.testOnly.skipReversalCrash, transferID)
		return true
	}
	return false
}

// SimulateCrashBeforeReversalAttempt marks a transfer so the reversal path
// will skip attempting an earlier leg's reversal on whatever inline path
// eventually drives it, mirroring SimulateCrashBeforeForwardLegAttempt on
// the compensating side.
func (tc *TransferCoordinator) SimulateCrashBeforeReversalAttempt(transferID string) {
	tc.testOnly.mu.Lock()
	defer tc.testOnly.mu.Unlock()
	tc.testOnly.skipReversalCrash[transferID] = true
}
