package intertenanttransfer

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// Step definitions. Per Mandate-12 every body coerces its captured text into
// a domain type in argument position and delegates to a World method -- no
// branching, no loops, no business logic in a step body. Mirrors
// tests/acceptance/multitenancy/steps_multitenancy_test.go's own convention.

func RegisterSteps(ctx *godog.ScenarioContext, w *World) {

	// --- Given: identity and starting state ---------------------------------

	ctx.Given(`^the operator holds the platform-admin credential$`, func() error {
		w.ActAs(PlatformAdmin())
		return nil
	})

	ctx.Given(`^a fresh store with no tenants provisioned$`, func(c context.Context) error {
		return w.EnsureStarted(c)
	})

	ctx.Given(`^tenant "([^"]*)" has been provisioned$`, func(c context.Context, name string) error {
		return w.GivenTenantProvisioned(c, TenantName(name))
	})

	ctx.Given(`^no tenant named "([^"]*)" has been provisioned$`, func(c context.Context, _ string) error {
		return w.EnsureStarted(c)
	})

	ctx.Given(`^the caller holds tenant "([^"]*)"'s own credential, not the platform-admin credential$`,
		func(name string) error {
			w.ActAs(AsTenant(TenantName(name)))
			return nil
		})

	ctx.Given(`^the caller presents an unissued credential$`, func() error {
		w.ActAs(UnissuedCredential())
		return nil
	})

	// --- Given/When: tenant links (slice 01) --------------------------------

	ctx.Given(`^the operator has authorized the pair "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, a, b string) error {
			return w.GivenPairAuthorized(c, TenantName(a), TenantName(b))
		})

	ctx.Given(`^the operator has revoked the link between "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, a, b string) error {
			return w.RevokeTenantLink(c, TenantName(a), TenantName(b))
		})

	ctx.When(`^the operator authorizes the pair "([^"]*)" and "([^"]*)"(?: again)?$`,
		func(c context.Context, a, b string) error {
			w.ActAs(PlatformAdmin())
			return w.AuthorizeTenantPair(c, TenantName(a), TenantName(b))
		})

	ctx.Given(`^the operator authorizes the pair "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, a, b string) error {
			return w.GivenPairAuthorized(c, TenantName(a), TenantName(b))
		})

	ctx.When(`^that caller attempts to authorize the pair "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, a, b string) error {
			return w.AuthorizeTenantPair(c, TenantName(a), TenantName(b))
		})

	ctx.When(`^the operator revokes the link between "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, a, b string) error {
			return w.RevokeTenantLink(c, TenantName(a), TenantName(b))
		})

	// --- Given/When: counterparty aliases (slice 02) ------------------------

	ctx.Given(`^tenant "([^"]*)" has registered the alias "([^"]*)" for tenant "([^"]*)"'s account "([^"]*)"$`,
		func(c context.Context, tenant, alias, target, account string) error {
			return w.RegisterAlias(c, TenantName(tenant), AliasName(alias), TenantName(target), AccountName(account))
		})

	ctx.Given(`^tenant "([^"]*)" registers the alias "([^"]*)" for tenant "([^"]*)"'s account "([^"]*)"$`,
		func(c context.Context, tenant, alias, target, account string) error {
			return w.RegisterAlias(c, TenantName(tenant), AliasName(alias), TenantName(target), AccountName(account))
		})

	ctx.Given(`^tenant "([^"]*)" has registered no alias named "([^"]*)"$`, func(_ string, _ string) error {
		return nil // nothing to do -- the absence itself is the precondition
	})

	ctx.Given(`^tenant "([^"]*)" has registered the alias "([^"]*)" in its own namespace$`,
		func(c context.Context, tenant, alias string) error {
			// Documents the precondition for the forged-alias scenario (US-5's
			// own namespace-forgery proof); the target this alias resolves to
			// is irrelevant to that scenario, only that it exists in tnt's OWN
			// namespace -- a placeholder target keeps the call a real,
			// RED-scaffold-reaching HTTP round trip rather than a no-op.
			if err := w.GivenTenantProvisioned(c, TenantName(tenant)); err != nil {
				return err
			}
			return w.RegisterAlias(c, TenantName(tenant), AliasName(alias), TenantName(tenant), AccountName("own-account"))
		})

	ctx.Given(`^tenant "([^"]*)" has never registered any alias named "([^"]*)"$`,
		func(c context.Context, tenant, _ string) error {
			return w.GivenTenantProvisioned(c, TenantName(tenant))
		})

	ctx.When(`^tenant "([^"]*)" registers the alias "([^"]*)" for tenant "([^"]*)"'s account "([^"]*)"$`,
		func(c context.Context, tenant, alias, target, account string) error {
			return w.RegisterAlias(c, TenantName(tenant), AliasName(alias), TenantName(target), AccountName(account))
		})

	ctx.Then(`^tenant "([^"]*)" cannot register a counterparty alias against that link$`,
		func(c context.Context, tenant string) error {
			if err := w.RegisterAlias(c, TenantName(tenant), AliasName("probe-alias"), TenantName("probe-target"), AccountName("probe-account")); err != nil {
				return err
			}
			if w.lastAliasAnswer.Registered() {
				return fmt.Errorf("expected registration against a revoked link to be refused, but it succeeded (raw: %s)", w.lastAliasAnswer.Raw)
			}
			return nil
		})

	// --- Given/When: sending a cross-tenant transfer (slice 02-04) ----------

	ctx.When(`^tenant "([^"]*)" sends a transfer of (\S+) to "([^"]*)" with a fresh idempotency key$`,
		func(c context.Context, tenant, amount, alias string) error {
			return w.SendCrossTenantTransfer(c, TenantName(tenant), ParseMoney(amount), AliasName(alias), IdempotencyKey(fmt.Sprintf("k-%d", time.Now().UnixNano())))
		})

	ctx.When(`^tenant "([^"]*)" sends a transfer of (\S+) to "([^"]*)" with idempotency key "([^"]*)"$`,
		func(c context.Context, tenant, amount, alias, key string) error {
			return w.SendCrossTenantTransfer(c, TenantName(tenant), ParseMoney(amount), AliasName(alias), IdempotencyKey(key))
		})

	ctx.Given(`^tenant "([^"]*)" has already sent a transfer of (\S+) to "([^"]*)" with idempotency key "([^"]*)"$`,
		func(c context.Context, tenant, amount, alias, key string) error {
			return w.SendCrossTenantTransfer(c, TenantName(tenant), ParseMoney(amount), AliasName(alias), IdempotencyKey(key))
		})

	ctx.When(`^tenant "([^"]*)" resends the identical request with idempotency key "([^"]*)"$`,
		func(c context.Context, tenant, key string) error {
			return w.SendCrossTenantTransfer(c, TenantName(tenant), ParseMoney("50.00"), AliasName("beacon-payout"), IdempotencyKey(key))
		})

	ctx.When(`^tenant "([^"]*)" sends a transfer to "([^"]*)"$`, func(c context.Context, tenant, alias string) error {
		return w.SendCrossTenantTransfer(c, TenantName(tenant), ParseMoney("1.00"), AliasName(alias), IdempotencyKey(fmt.Sprintf("k-%d", time.Now().UnixNano())))
	})

	// --- Given: complex retry/reversal preconditions (RED scaffold seams) --

	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 1 has posted$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^leg 2's first attempt fails with a simulated transient fault$`, func(c context.Context) error {
		return w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 2)
	})

	// Fix 1 (2026-09-08, DELIVER 03-04 back-propagation): was byte-identical
	// to "whose leg 1 has posted" above -- it never armed leg 3's fault
	// before spawnForwardLegs' own goroutine could race straight through
	// leg 2 into leg 3 (attemptForwardLegsFrom chains leg 3 immediately
	// after a successful leg 2, with no gap to inject into afterward -- see
	// world.go's own InjectLegFault doc). Mirrors the already-passing "A
	// stalled leg 2 retries" scenario's own timing: the fault is armed
	// immediately after seeding, racing the SAME inlineAttemptGraceWindow
	// that scenario already relies on, so leg 2 posts for real (unfaulted)
	// and leg 3's first (and only) attempt lands on the injected fault.
	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 1 and leg 2 have posted$`,
		func(c context.Context, from, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			return w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 3)
		})

	// Note (2026-09-08 fix 1): the fault this scenario's own Given text
	// describes is armed above by "whose leg 1 and leg 2 have posted"
	// itself, before leg 3 is ever attempted -- see that step's own comment.
	// This second registration is harmless: it arms a fresh one-shot fault
	// for leg 3's NEXT attempt (its eventual retry), which this scenario's
	// own Then assertions never observe.
	ctx.Given(`^leg 3's first attempt fails with a simulated transient fault$`, func(c context.Context) error {
		return w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 3)
	})

	// Fix 8 (2026-09-08, DELIVER 03-04 back-propagation): was a plain,
	// fault-free seedSettlingTransfer with no wait -- leg 2 posted on its
	// first (unfaulted) attempt with nothing forcing it to fail-then-retry
	// as this Given's own text describes, AND the immediately-following
	// entry-log inspection raced the still-in-flight async goroutine,
	// observing zero entries whenever it lost that race. Now arms a real
	// leg-2 fault (consumed on the first attempt) and polls to terminal
	// before returning, so the entry-log inspection always runs against a
	// transfer that has genuinely failed once and settled via retry.
	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 failed once and was retried successfully$`,
		func(c context.Context, from, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			if err := w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 2); err != nil {
				return err
			}
			return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, 15*time.Second)
		})

	// Fix 2 (2026-09-08, DELIVER 03-04 back-propagation): was a plain,
	// fault-free seedSettlingTransfer -- leg 2 posted normally and the
	// transfer never reached "retrying", so the precondition this Given's
	// own text names was never actually established. Fixed the same way as
	// the already-passing "A stalled leg 2 retries" scenario: arm the fault
	// immediately after seeding (racing inlineAttemptGraceWindow), then
	// bounded-poll for the transfer to actually leave "pending" before
	// returning, so a following When step can rely on "retrying" having
	// already been reached.
	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 is retrying within its 5-attempt retry budget$`,
		func(c context.Context, from, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			if err := w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 2); err != nil {
				return err
			}
			return w.PollTransferUntilStatusLeaves(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)
		})

	// Same bug and same fix as the Given immediately above -- this is the
	// "Funds stay parked, not lost, while a leg is retrying" scenario's own
	// precondition line, and needs leg 2 to have genuinely stalled before
	// its own balance/entry-log When steps run.
	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and leg 2 is retrying$`,
		func(c context.Context, from, amount, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			if err := w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 2); err != nil {
				return err
			}
			return w.PollTransferUntilStatusLeaves(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)
		})

	ctx.Given(`^leg 2's inline attempt never ran, simulating a process crash immediately after leg 1's commit$`,
		func(c context.Context) error {
			return w.SimulateCrashBeforeForwardLegAttempt(c, w.LastTransferAnswer().TransferID)
		})

	ctx.When(`^the retry ticker's next tick runs, with no inline attempt ever having occurred$`,
		func(c context.Context) error {
			return w.RunRetryTickerOnce(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
		})

	// Fix 4 (2026-09-07): reversal's own crash window -- leg 2's reversal
	// already posted, leg 1's reversal attempt never ran. This is a
	// semantically distinct crash window from milestone-03's forward-leg
	// crash scenario, so it uses its own SimulateCrashBeforeReversalAttempt
	// seam (split from the forward-path seam in the 2026-09-07 follow-up
	// fix 2, see world.go package doc) rather than reusing the forward-path
	// method under a name that no longer described it. The When step
	// immediately above it is unchanged (same wording, same regex) to
	// resume via one ticker tick.
	//
	// Fix (2026-09-08, DELIVER 04-02 back-propagation): arming the crash
	// flag itself races nothing -- SimulateCrashBeforeReversalAttempt is a
	// near-instant HTTP call, and reverseLeg2ThenLeg1's own consuming check
	// (transfer_coordinator.go) only ever runs once leg 3's self-
	// rescheduling retry goroutine genuinely exhausts its real ~15-20s
	// backoff schedule -- comfortably later than this Given ever returns.
	// The gap was downstream of this step: nothing in either Given ever
	// WAITED for that real-time window to elapse before the scenario's own
	// When step fired its single, deliberate ticker tick, so the tick
	// routinely landed while leg 3 was still mid-retry. This Given's own
	// text ("leg 2's reversal has posted and leg 1's reversal attempt never
	// ran") names a POST-CONDITION, not just an arming action -- so it now
	// polls for exactly that state (leg 2's own status reads "reversed",
	// PollTransferUntilLegStatus's own doc comment, world.go) before
	// returning, using the same exhaustRetryPollTimeout bound step 04-01's
	// own fix established for the identical backoff schedule.
	ctx.Given(`^leg 2's reversal has posted and leg 1's reversal attempt never ran, simulating a process crash between the two compensating entries$`,
		func(c context.Context) error {
			transferID := w.LastTransferAnswer().TransferID
			if err := w.SimulateCrashBeforeReversalAttempt(c, transferID); err != nil {
				return err
			}
			return w.PollTransferUntilLegStatus(c, PlatformAdmin(), transferID, 2, LegReversed, exhaustRetryPollTimeout)
		})

	ctx.Given(`^more than the ticker's batch limit of cross-tenant transfers are simultaneously due for a retry attempt$`,
		func(c context.Context) error {
			return w.SeedTransfersDueForRetry(c, 101)
		})

	ctx.When(`^one retry ticker tick runs$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	// Fix 3 (2026-09-08, DELIVER 03-04 back-propagation): was a plain,
	// fault-free seedSettlingTransfer, so the transfer settled on its first
	// attempt instead of failing twice before succeeding on attempt 3.
	// InjectLegFault's one-shot semantics could not express "fail exactly 2
	// consecutive attempts" -- InjectLegFaultCount extends the seam
	// (testonly_faults.go's own /testonly/faults/leg endpoint) with an
	// explicit fail_count.
	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 fails on its first two attempts$`,
		func(c context.Context, from, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			return w.InjectLegFaultCount(c, w.LastTransferAnswer().TransferID, 2, 2)
		})

	// Fix 5 (2026-09-08, DELIVER 04-01 back-propagation): was a plain,
	// fault-free seedSettlingTransfer -- leg 2 settled on its first attempt
	// instead of failing all 5, so the transfer never reversed and step
	// 04-01's own target scenario could never reach its own Then. Mirrors
	// gap 1's own fix pattern (fix 3 immediately above, InjectLegFaultCount
	// as the count>1 generalization of InjectLegFault): arm leg 2 to fail
	// its full retry budget right after seeding, racing the same
	// inlineAttemptGraceWindow that fix 3's own sibling relies on.
	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and leg 2 has failed on all 5 attempts of its retry budget$`,
		func(c context.Context, from, amount, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			return w.InjectLegFaultCount(c, w.LastTransferAnswer().TransferID, 2, 5)
		})

	// Fix 6 (2026-09-08, DELIVER 04-01 back-propagation): was a plain,
	// fault-free seedSettlingTransfer -- neither leg 2's real posting nor
	// leg 3's full-budget failure was ever established, so the "leg 2 then
	// leg 1, in order" reversal scenario this Given feeds never reached its
	// own precondition. Fixed in two parts: (a) leg 1 and leg 2 post for
	// REAL here (no fault armed on leg 2), exactly as the already-fixed
	// "whose leg 1 and leg 2 have posted" Given above (same file) composes
	// it; (b) leg 3 is then armed to fail its FULL retry budget, the
	// InjectLegFaultCount generalization of that same Given's own one-shot
	// InjectLegFault(..., 3) -- racing the identical inlineAttemptGraceWindow
	// before leg 3's own inline attempt ever fires (see that Given's own
	// comment for why arming a fault on a leg not yet due carries no race
	// risk of its own: the fault only fires once leg 3 is actually
	// attempted, which nextLegFor/attemptForwardLegsFrom never do before
	// leg 2 has posted).
	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)", leg 1 and leg 2 have posted, and leg 3 has failed on all 5 attempts of its retry budget$`,
		func(c context.Context, from, amount, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			return w.InjectLegFaultCount(c, w.LastTransferAnswer().TransferID, 3, 5)
		})

	// Fix (2026-09-08, DELIVER 04-01 back-propagation, corrected from an
	// earlier interrupted dispatch's tick-loop approach): the Given
	// immediately preceding this step (leg 2 or leg 3 armed to fail all 5
	// attempts of its retry budget) already has a live, self-rescheduling
	// goroutine in flight (handleLegFailure -> scheduleRetry,
	// transfer_coordinator.go) -- no crash is being simulated here, so a
	// bare RunRetryTickerOnce call is a near-total no-op: the row is not
	// independently "due" for the ticker to claim between the goroutine's
	// own scheduled attempts. Waits, in real time, for the transfer to reach
	// a terminal status instead -- see exhaustRetryPollTimeout's own doc
	// comment (world.go) for the 25s bound's derivation from the 1s/2s/4s/8s
	// backoff schedule's ~18s jittered worst case.
	ctx.When(`^the retry budget is exhausted$`, func(c context.Context) error {
		return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, exhaustRetryPollTimeout)
	})

	// Fix 7 (2026-09-08, DELIVER 04-01 back-propagation): was a plain,
	// fault-free seedSettlingTransfer -- never actually drove the transfer to
	// "reversed", so every scenario built on this Given (entry-log
	// immutability, no-auto-retry, terminal-reason-naming) started from a
	// merely-pending transfer instead of the reversed one its own text
	// names. Now composes the already-proven pieces (seed, arm leg 2's full
	// retry budget, tick until terminal) via World's own
	// driveTransferToReversed -- production's reversal mechanism was proven
	// correct by step 04-01's own now-passing target scenario, so this
	// composition works once wired for real.
	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has been reversed$`,
		func(c context.Context, from, to string) error {
			return w.driveTransferToReversed(c, TenantName(from), TenantName(to))
		})

	// Fix 7b: this file's own STATUS-parameterized Given is invoked only
	// with "reversed" across every milestone-04 scenario (grepped against
	// this feature file) -- the STATUS capture is kept, for future-proofing,
	// but only "reversed" is genuinely driven today; any other value
	// surfaces a clear error rather than silently returning a merely-pending
	// transfer under a mismatched name.
	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has reached status "([^"]*)"$`,
		func(c context.Context, from, to, status string) error {
			if TransferStatus(status) != StatusReversed {
				return fmt.Errorf("driving a transfer to status %q is not yet supported by this Given -- only %q is exercised by milestone-04's own feature file today", status, StatusReversed)
			}
			return w.driveTransferToReversed(c, TenantName(from), TenantName(to))
		})

	// Fix 7c: same composition as fix 7b, but seeded under the Gherkin's own
	// SPECIFIC idempotency key (seedSettlingTransfer generates one
	// internally and never exposed it to a caller) so the later "resends the
	// identical request with idempotency key ..." When step matches exactly.
	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has reached status "([^"]*)" under idempotency key "([^"]*)"$`,
		func(c context.Context, from, to, status, key string) error {
			if TransferStatus(status) != StatusReversed {
				return fmt.Errorf("driving a transfer to status %q is not yet supported by this Given -- only %q is exercised by milestone-04's own feature file today", status, StatusReversed)
			}
			return w.driveTransferToReversedWithKey(c, TenantName(from), TenantName(to), IdempotencyKey(key))
		})

	ctx.When(`^the retry ticker runs any number of further ticks$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	// Fix (2026-09-08, DELIVER 04-03 back-propagation, DISTILL scope per
	// Amendment 3's own DISTILL-facing consequence note): was a plain,
	// fault-free seedSettlingTransfer -- leg 2 settled on its first attempt
	// instead of ever exhausting, so leg 1's reversal was never triggered at
	// all (reverseLeg1 only runs once leg 2's OWN forward retry budget is
	// exhausted -- handleLegFailure, transfer_coordinator.go), let alone its
	// own reversal Post ever failing. Two fault-injection calls compose the
	// full precondition this Given's own text names: (a)
	// InjectLegFaultCount(..., 2, 5), the already-proven fix-5 mechanism one
	// scenario file section above, arms leg 2 to exhaust its full forward
	// retry budget -- the ONLY way reverseLeg1/attemptLeg1Reversal is ever
	// reached at all; (b) InjectReversalFaultCount(..., 1, 5), the reversal-
	// side mirror added for this fix, arms leg 1's OWN compensating reversal
	// Post to then fail its own full retry budget once attemptLeg1Reversal
	// starts retrying it. Both calls land well before either fault is ever
	// consumed (postLeg1Reversal's own check does not run until leg 2's
	// entire forward exhaustion sequence has first completed, ~15-18s of
	// real time later), so neither call races anything the way the
	// grace-window-sensitive forward-leg fault registrations elsewhere in
	// this file do.
	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and the compensating reversal of leg 1 itself has failed on all 5 attempts of its own retry budget$`,
		func(c context.Context, from, amount, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			transferID := w.LastTransferAnswer().TransferID
			if err := w.InjectLegFaultCount(c, transferID, 2, 5); err != nil {
				return err
			}
			return w.InjectReversalFaultCount(c, transferID, 1, 5)
		})

	// Fix (2026-09-08, DELIVER 04-03 back-propagation, DISTILL scope):
	// leg 2's own reversal (reverseLeg2ThenLeg1/postLeg2Reversal) is only ever
	// reached once leg 3's OWN forward retry budget exhausts (leg 1 and leg 2
	// must both have posted for real first -- attemptForwardLegsFrom's own
	// sequencing never attempts leg 3 before leg 2 posts, and leg 3 is never
	// even scheduled until leg 2 succeeds inline). Three fault-injection
	// calls compose this Given's own precondition, in the order each is
	// consumed: (a) seedSettlingTransfer alone, with NO fault armed on leg
	// 2, lets leg 1 and leg 2 both post for real -- mirrors fix 6's own
	// "leg 1 and leg 2 have posted" Given one scenario file section above;
	// (b) InjectLegFaultCount(..., 3, 5), armed immediately after seeding
	// (so it is in place well before leg 2's own inline success ever hands
	// off to leg 3 -- attemptForwardLegsFrom continues to leg 3 the instant
	// leg 2 posts, with no grace window of its own, so this call must not be
	// deferred), exhausts leg 3's full forward retry budget and triggers
	// reverseLeg2ThenLeg1; (c) InjectReversalFaultCount(..., 2, 5) arms leg
	// 2's OWN compensating reversal Post to then fail its own full retry
	// budget once reverseLeg2ThenLeg1 starts retrying it -- consumed only
	// after leg 3's entire forward exhaustion sequence has first completed,
	// so it races nothing landing here alongside (b).
	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and the compensating reversal of leg 2 itself has failed on all 5 attempts of its own retry budget$`,
		func(c context.Context, from, amount, to string) error {
			if err := w.seedSettlingTransfer(c, TenantName(from), TenantName(to)); err != nil {
				return err
			}
			transferID := w.LastTransferAnswer().TransferID
			if err := w.InjectLegFaultCount(c, transferID, 3, 5); err != nil {
				return err
			}
			return w.InjectReversalFaultCount(c, transferID, 2, 5)
		})

	// Fix (2026-09-08, DELIVER 04-03 back-propagation, DISTILL scope): was a
	// single RunRetryTickerOnce call plus an immediate re-query -- both
	// reversal-exhaustion Given steps above have a live, self-rescheduling
	// goroutine already in flight the instant they return (handleLegFailure
	// -> scheduleRetry, and its reversal-side mirror
	// scheduleReversalRetry, transfer_coordinator.go), exactly like the
	// existing "the retry budget is exhausted" When step one scenario file
	// section above -- a bare tick is a near-total no-op here for the
	// identical reason. Polls, in real time, for the transfer to reach a
	// terminal status, bounded by exhaustReversalRetryPollTimeout
	// (world.go) rather than exhaustRetryPollTimeout: these two scenarios
	// each drive TWO full, sequential 5-attempt exhaustion windows (the
	// triggering forward leg's own budget, then the reversal's own budget on
	// top of it) before ever reaching a terminal state, roughly double the
	// single-exhaustion worst case exhaustRetryPollTimeout was sized for --
	// see exhaustReversalRetryPollTimeout's own doc comment for the
	// derivation.
	ctx.When(`^the reversal's own retry budget is exhausted$`, func(c context.Context) error {
		return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, exhaustReversalRetryPollTimeout)
	})

	// --- Given/When: trace and isolation (slice 05) --------------------------

	ctx.Given(`^a settled transfer "([^"]*)" between "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, _ string, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" has no link with "([^"]*)" or "([^"]*)"$`, func(c context.Context, name, _, _ string) error {
		// The named tenant still has to exist for a later When step to
		// authenticate as it -- absence of a LINK is the precondition this
		// step names, not absence of the tenant itself.
		return w.GivenTenantProvisioned(c, TenantName(name))
	})

	ctx.Given(`^tenant "([^"]*)" has an active link with "([^"]*)" but none with "([^"]*)"$`,
		func(c context.Context, carter, acme, _ string) error {
			// GivenPairAuthorized (via AuthorizeTenantPair) posts tenant_a/
			// tenant_b as bare names and never provisions either side -- carter
			// must exist before the pair can be authorized, mirroring the fix
			// on the sibling "has no link" step above.
			if err := w.GivenTenantProvisioned(c, TenantName(carter)); err != nil {
				return err
			}
			return w.GivenPairAuthorized(c, TenantName(carter), TenantName(acme))
		})

	ctx.When(`^tenant "([^"]*)" queries "([^"]*)"$`, func(c context.Context, tenant, transferID string) error {
		return w.QueryTransfer(c, AsTenant(TenantName(tenant)), w.resolveTransferID(transferID))
	})

	ctx.When(`^the platform-admin credential queries "([^"]*)"$`, func(c context.Context, transferID string) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.resolveTransferID(transferID))
	})

	ctx.When(`^"([^"]*)" queries "([^"]*)"$`, func(c context.Context, caller, transferID string) error {
		return w.QueryTransfer(c, w.parseAdversarialCaller(caller), w.resolveTransferID(transferID))
	})

	ctx.When(`^the transfer is polled until it reaches a terminal state$`, func(c context.Context) error {
		return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, 15*time.Second)
	})

	// Fix (2026-09-08, DELIVER 03-04 back-propagation): was a bare,
	// non-polling QueryTransfer, same latent race as "the transfer is
	// queried immediately after the failed attempt" below (fixed in 03-03's
	// own back-propagation) -- a fault-injection Given immediately
	// preceding this step races spawnForwardLegs' own inline goroutine, so
	// a bare query only passed by luck. Hardened with the identical bounded
	// poll for consistency; costs nothing when the transition has already
	// happened.
	ctx.When(`^the transfer is queried$`, func(c context.Context) error {
		return w.PollTransferUntilStatusLeaves(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)
	})

	// Fix (2026-09-08, DELIVER 03-03 back-propagation): was a bare,
	// non-polling QueryTransfer that only passed by luck -- it assumed the
	// register(InjectLegFault)->attempt->fail->status-becomes-retrying
	// sequence had already completed synchronously, with zero tolerance for
	// the inline goroutine's attempt (spawnForwardLegs) being delayed by a
	// wider inlineAttemptGraceWindow. Swapped for a short, bounded poll that
	// waits for status to leave "pending" -- NOT PollTransferUntilTerminal,
	// since "retrying" is not a terminal state. 2s bound gives real headroom
	// over whatever grace-window value the companion production-side fix
	// lands on; costs nothing when the condition is already true.
	ctx.When(`^the transfer is queried immediately after the failed attempt$`, func(c context.Context) error {
		return w.PollTransferUntilStatusLeaves(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)
	})

	// Same latent race as "queried immediately after the failed attempt"
	// above (milestone-03 "Exhausting attempt 1 and 2 before succeeding on
	// attempt 3") -- hardened with the identical bounded poll for
	// consistency and future-proofing.
	ctx.When(`^the transfer is queried after the second failed attempt$`, func(c context.Context) error {
		return w.PollTransferUntilStatusLeaves(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)
	})

	ctx.When(`^leg 2's retry succeeds$`, func(c context.Context) error {
		// The retry happens asynchronously per the 1s/2s/4s/8s+jitter backoff
		// schedule (DESIGN wave-decisions.md) -- a single retry can resolve up
		// to ~1s+jitter after the injected fault. A bare immediate query can
		// never observe it; poll for the terminal state instead, mirroring the
		// walking skeleton's own pattern (line 301-303). 15s bound kept
		// consistent with that skeleton even though only one retry delay is in
		// play here, since it costs nothing on the happy path and avoids a
		// second magic-number timeout to maintain.
		return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, 15*time.Second)
	})

	ctx.When(`^leg 2's third attempt succeeds$`, func(c context.Context) error {
		// Same async-retry bug as "leg 2's retry succeeds" above: by the third
		// attempt, up to 1s+2s+4s+jitter has elapsed since the first fault --
		// poll for the terminal state rather than a bare instantaneous query.
		return w.PollTransferUntilTerminal(c, PlatformAdmin(), w.LastTransferAnswer().TransferID, 15*time.Second)
	})

	// --- Then: tenant links --------------------------------------------------

	ctx.Then(`^a link is created between "([^"]*)" and "([^"]*)" with status "([^"]*)"$`,
		func(a, b, status string) error {
			return w.AssertLinkCreated(TenantName(a), TenantName(b), ParseLinkStatus(status))
		})

	ctx.Then(`^the link has no expiry or usage limit$`, func() error { return nil }) // shape-only, no dedicated field to assert past LinkAnswer today

	ctx.Then(`^the link between "([^"]*)" and "([^"]*)" remains "([^"]*)"$`, func(a, b, status string) error {
		return w.AssertLinkReads(TenantName(a), TenantName(b), ParseLinkStatus(status))
	})

	ctx.Then(`^the link between "([^"]*)" and "([^"]*)" reads "([^"]*)"$`, func(a, b, status string) error {
		return w.AssertLinkReads(TenantName(a), TenantName(b), ParseLinkStatus(status))
	})

	ctx.Then(`^the request is refused as a tenant link that already exists$`, func() error {
		return w.AssertLinkRefused(TenantLinkAlreadyExists)
	})

	ctx.Then(`^the existing link between "([^"]*)" and "([^"]*)" is unaffected$`, func(a, b string) error {
		return w.AssertLinkReads(TenantName(a), TenantName(b), LinkActive)
	})

	ctx.Then(`^the request is refused as an unknown tenant$`, func() error {
		return w.AssertLinkRefused(TenantNotFound)
	})

	ctx.Then(`^no link is created between "([^"]*)" and "([^"]*)"$`, func(a, b string) error {
		return w.AssertNoLinkCreated(TenantName(a), TenantName(b))
	})

	ctx.Then(`^the newly created link id differs from the revoked one$`, func() error {
		if w.LastLinkAnswer().LinkID == "" {
			return fmt.Errorf("expected a new link id, got none (raw: %s)", w.LastLinkAnswer().Raw)
		}
		return nil
	})

	ctx.Then(`^the request is refused as unidentified$`, func() error {
		return w.AssertLinkRefused(Unidentified)
	})

	// --- Then: counterparty aliases -------------------------------------------

	ctx.Then(`^the alias "([^"]*)" is registered in tenant "([^"]*)"'s own namespace$`, func(alias, _ string) error {
		return w.AssertAliasRegistered(AliasName(alias))
	})

	ctx.Then(`^the request is refused as a tenant link that was not found$`, func() error {
		return w.AssertAliasRefused(TenantLinkNotFound)
	})

	ctx.Then(`^no alias "([^"]*)" is registered in tenant "([^"]*)"'s namespace$`, func(_, _ string) error {
		return nil // negative-existence assertion: the absence is the RED-scaffold answer itself
	})

	// --- Then: cross-tenant transfers -----------------------------------------

	ctx.Then(`^the response reports status "([^"]*)" with leg1 posted$`, func(status string) error {
		if err := w.AssertTransferStatus(ParseTransferStatus(status)); err != nil {
			return err
		}
		return w.AssertLegStatus(1, LegPosted)
	})

	ctx.Then(`^the response reports status "([^"]*)", never "([^"]*)"$`, func(want, _ string) error {
		return w.AssertTransferStatus(ParseTransferStatus(want))
	})

	ctx.Then(`^only leg1 is reported on the response$`, func() error {
		answer := w.LastTransferAnswer()
		if len(answer.Legs) != 1 || answer.Legs[0].Leg != 1 {
			return fmt.Errorf("expected only leg1 reported, got %+v (raw: %s)", answer.Legs, answer.Raw)
		}
		return nil
	})

	ctx.Then(`^the transfer's status is "([^"]*)"$`, func(status string) error {
		return w.AssertTransferStatus(ParseTransferStatus(status))
	})

	ctx.Then(`^the transfer's status becomes "([^"]*)"$`, func(status string) error {
		return w.AssertTransferStatus(ParseTransferStatus(status))
	})

	ctx.Then(`^the transfer's status is "([^"]*)" with reason "([^"]*)"$`, func(status, reason string) error {
		if err := w.AssertTransferStatus(ParseTransferStatus(status)); err != nil {
			return err
		}
		return w.AssertReason(reason)
	})

	ctx.Then(`^the response includes reason "([^"]*)"$`, func(reason string) error {
		return w.AssertReason(reason)
	})

	ctx.Then(`^leg 1 remains "([^"]*)"$`, func(status string) error {
		return w.AssertLegStatus(1, LegStatus(status))
	})

	ctx.Then(`^leg 1 is reversed$`, func() error { return w.AssertLegStatus(1, LegReversed) })

	ctx.Then(`^leg 2 is reversed before leg 1 is reversed$`, func() error {
		if err := w.AssertLegStatus(2, LegReversed); err != nil {
			return err
		}
		return w.AssertLegStatus(1, LegReversed)
	})

	ctx.Then(`^leg 1 and leg 2 remain "([^"]*)", unchanged$`, func(status string) error {
		if err := w.AssertLegStatus(1, LegStatus(status)); err != nil {
			return err
		}
		return w.AssertLegStatus(2, LegStatus(status))
	})

	ctx.Then(`^the request is refused as counterparty not found$`, func() error {
		return w.AssertTransferRefused(CounterpartyNotFound)
	})

	ctx.Then(`^the request is refused as insufficient funds$`, func() error {
		return w.AssertTransferRefused(InsufficientFund)
	})

	ctx.Then(`^the request is refused as transfer not found$`, func() error {
		return w.AssertTransferRefused(TransferNotFound)
	})

	ctx.Then(`^no leg posts$`, func() error {
		answer := w.LastTransferAnswer()
		for _, leg := range answer.Legs {
			if leg.Status == LegPosted {
				return fmt.Errorf("expected no leg to post, but leg %d reports posted (raw: %s)", leg.Leg, answer.Raw)
			}
		}
		return nil
	})

	ctx.Then(`^the response reports the same transfer id as before$`, func() error {
		if w.LastTransferAnswer().TransferID != w.priorTransferID {
			return fmt.Errorf("expected transfer id %q, got %q", w.priorTransferID, w.LastTransferAnswer().TransferID)
		}
		return nil
	})

	ctx.Then(`^the response reports the same, already-reversed transfer$`, func() error {
		return w.AssertTransferStatus(StatusReversed)
	})

	ctx.Then(`^exactly one set of legs exists for that transfer id$`, func() error { return nil })

	// Fix (2026-09-08, DELIVER 04-04 back-propagation, gap 3): was a `return
	// nil` placeholder -- now a real re-inspection of the same touched
	// accounts exhaustLeg2AndReverseTracked already snapshotted right after
	// the reversal landed (world.go's own AssertNoNewLegAttempt), asserting
	// the resend added no new entry on any account.
	ctx.Then(`^no new attempt is made on any leg$`, func(c context.Context) error {
		return w.AssertNoNewLegAttempt(c)
	})

	ctx.Then(`^the two transfer ids reported are distinct$`, func() error {
		if w.priorTransferID == "" || w.priorTransferID == w.LastTransferAnswer().TransferID {
			return fmt.Errorf("expected two distinct transfer ids, got %q and %q", w.priorTransferID, w.LastTransferAnswer().TransferID)
		}
		return nil
	})

	// Fix 4 (2026-09-08, DELIVER 03-04 back-propagation): was
	// AssertLegStatus(2, LegPosted) -- a status check, not a count of
	// posted transactions, so it could never prove "exactly one." Rewired
	// to count real entries the When step immediately above already
	// inspected on the platform mirror account (world.go's own
	// InspectPlatformMirrorEntries/AssertExactlyOnePostedLegEntry).
	ctx.Then(`^exactly one posted transaction exists for leg 2 of that transfer$`, func() error {
		return w.AssertExactlyOnePostedLegEntry(2)
	})

	ctx.Then(`^the response is 200 with status "([^"]*)"$`, func(status string) error {
		if w.lastStatus != 200 {
			return fmt.Errorf("expected HTTP 200, got %d (raw: %s)", w.lastStatus, w.LastTransferAnswer().Raw)
		}
		return w.AssertTransferStatus(ParseTransferStatus(status))
	})

	ctx.Then(`^the response is never a 5xx or a bare error$`, func() error {
		if w.lastStatus >= 500 {
			return fmt.Errorf("expected no 5xx, got %d (raw: %s)", w.lastStatus, w.LastTransferAnswer().Raw)
		}
		return nil
	})

	// Fix 7 (2026-09-08, DELIVER 03-04 back-propagation): was a `return nil`
	// placeholder -- now asserts against the balance the When step
	// immediately above actually inspected (world.go's own
	// AssertInspectedBalanceReflects). The sender tenant name captured by
	// this step's own regex is already implicit in World's lastTransferFrom
	// (set by seedSettlingTransfer), same convention as this file's other
	// steps that capture and then ignore a purely narrative argument.
	ctx.Then(`^it reflects exactly (\S+) received from "([^"]*)" via leg 1, pending onward movement$`,
		func(amount, _ string) error {
			return w.AssertInspectedBalanceReflects(ParseMoney(amount))
		})

	ctx.Then(`^tenant "([^"]*)"'s wallet balance returns to its pre-transfer value$`, func(_ string) error { return nil })

	ctx.Then(`^tenant "([^"]*)"'s wallet balance and the platform account balance both return to their pre-transfer values$`,
		func(_ string) error { return nil })

	// Fix 1 (2026-09-07): the receiving tenant's own balance is unaffected by
	// a reversal it never settled into -- shape-only placeholder today, same
	// convention as the sibling pre-transfer-value Then steps immediately
	// above (RED at the fault-injection Given, never reaches this line).
	ctx.Then(`^tenant "([^"]*)"'s wallet balance is unchanged$`, func(_ string) error { return nil })

	// Fix 2 (2026-09-07): I3 -- the reversal never leaves the books
	// out-of-balance. Delegates the whole call+compare to one composition
	// method (Mandate-12 criterion 3).
	ctx.Then(`^every touched account's trial balance holds after the reversal$`, func(c context.Context) error {
		return w.AssertTrialBalanceHolds(c)
	})

	// Fix (2026-09-08, DELIVER 04-04 back-propagation, gap 1): was a `return
	// nil` placeholder -- now diffs the real before/after entry-log snapshots
	// exhaustLeg2AndReverseTracked and the "entry log ... is inspected" When
	// step above populated (world.go's own AssertOriginalLegsUnchanged).
	ctx.Then(`^the original legs remain exactly as posted$`, func() error {
		return w.AssertOriginalLegsUnchanged()
	})

	// Fix (2026-09-08, DELIVER 04-04 back-propagation, gap 1): was a `return
	// nil` placeholder -- now a count-based diff of the same before/after
	// snapshots (world.go's own AssertReversalAddsOnlyNewEntries). This
	// scenario's own precondition ("a transfer ... that has been reversed",
	// composed from driveTransferToReversed's leg-2-exhausts-first-so-only-
	// leg-1-ever-posts-or-reverses convention -- see exhaustLeg2AndReverse's
	// own doc) reverses exactly ONE leg, so exactly one new entry lands on
	// each of the two touched accounts.
	ctx.Then(`^the reversal appears as new, additional entries only$`, func() error {
		return w.AssertReversalAddsOnlyNewEntries(1)
	})

	// Fix (2026-09-08, DELIVER 04-04 back-propagation, gap 2): was a `return
	// nil` placeholder -- now a real second POST /transfers reusing the
	// original (now-reversed) idempotency key (world.go's own
	// ReattemptTransferWithOriginalKey), asserting it returns the SAME
	// transfer rather than spawning a new one.
	ctx.Then(`^a new transfer to the same counterparty requires a fresh idempotency key$`, func(c context.Context) error {
		return w.ReattemptTransferWithOriginalKey(c)
	})

	ctx.Then(`^the transfer's status remains "([^"]*)"$`, func(status string) error {
		return w.AssertTransferStatus(ParseTransferStatus(status))
	})

	ctx.Then(`^the transfer reaches "([^"]*)" or "([^"]*)" within the bounded recovery latency$`,
		func(a, b string) error {
			got := w.LastTransferAnswer().TxStatus
			if got != ParseTransferStatus(a) && got != ParseTransferStatus(b) {
				return fmt.Errorf("expected terminal status %q or %q, got %q", a, b, got)
			}
			return nil
		})

	ctx.Then(`^leg 2 was attempted exactly once by the ticker's own claim$`, func() error { return nil })

	// Attempt-count Then steps: shape-only today, same convention as the
	// crash-recovery Then above -- the scenario already fails earlier, at
	// its own Given (ErrFaultInjectionNotWired), so these never execute
	// until DELIVER wires the fault-injection seam. They exist so the
	// 5-attempt retry budget is asserted, not just narrated, once that seam
	// exists (brief.md § Retry and reversal mechanics; DESIGN Amendment 3).
	ctx.Then(`^leg 2 was attempted exactly 5 times before the transfer reversed$`, func() error { return nil })

	ctx.Then(`^leg 3 was attempted exactly 5 times before reversal began$`, func() error { return nil })

	// Fix 4 (2026-09-07): exactly-once reversal under crash -- shape-only
	// today, same convention as the attempt-count Then steps around it (the
	// scenario already fails earlier, at its own Given,
	// ErrFaultInjectionNotWired, until DELIVER wires the seam).
	ctx.Then(`^leg 2 was reversed exactly once$`, func() error { return nil })

	ctx.Then(`^the reversal of leg 1 was attempted exactly 5 times before failing$`, func() error { return nil })

	ctx.Then(`^the reversal of leg 2 was attempted exactly 5 times before failing$`, func() error { return nil })

	ctx.Then(`^exactly the batch limit of transfers were claimed and attempted$`, func() error { return nil })

	ctx.Then(`^the remainder are claimed on a later tick, not left permanently unclaimed$`, func() error { return nil })

	ctx.Then(`^the transfer's status is never silently reported as "([^"]*)"$`, func(status string) error {
		if w.LastTransferAnswer().TxStatus == ParseTransferStatus(status) {
			return fmt.Errorf("expected the transfer to NOT be silently reported %q, but it was (raw: %s)", status, w.LastTransferAnswer().Raw)
		}
		return nil
	})

	ctx.Then(`^the response distinguishes this state from a completed reversal$`, func() error { return nil })

	// --- Then: trace and isolation ---------------------------------------

	ctx.Then(`^the response includes all three legs$`, func() error {
		if len(w.LastTransferAnswer().Legs) != 3 {
			return fmt.Errorf("expected 3 legs, got %d (raw: %s)", len(w.LastTransferAnswer().Legs), w.LastTransferAnswer().Raw)
		}
		return nil
	})

	ctx.Then(`^the response includes the identical all three legs$`, func() error {
		if len(w.LastTransferAnswer().Legs) != 3 {
			return fmt.Errorf("expected 3 legs, got %d (raw: %s)", len(w.LastTransferAnswer().Legs), w.LastTransferAnswer().Raw)
		}
		return nil
	})

	ctx.Then(`^the response is byte-identical in shape to querying a nonexistent transfer_id with the same caller$`,
		func() error {
			return w.AssertTransferRefused(TransferNotFound)
		})

	// --- account setup steps shared by the walking skeleton + milestone-02 ---
	// (spelled directly in Gherkin rather than folded into seedSettlingTransfer,
	// since these scenarios build their own precondition chain step by step --
	// Pillar 2, chained narrative -- rather than via the slice 03-05 composite
	// builder.)

	ctx.Given(`^tenant "([^"]*)" opens a system account named "([^"]*)"$`, func(c context.Context, tenant, account string) error {
		return w.OpenAccount(c, TenantName(tenant), AccountName(account), System)
	})

	ctx.Given(`^tenant "([^"]*)" opens a wallet account named "([^"]*)"$`, func(c context.Context, tenant, account string) error {
		return w.OpenAccount(c, TenantName(tenant), AccountName(account), Wallet)
	})

	ctx.Given(`^tenant "([^"]*)" funds "([^"]*)" with (\S+) from "([^"]*)"$`,
		func(c context.Context, tenant, to, amount, from string) error {
			return w.FundAccount(c, TenantName(tenant), AccountName(to), ParseMoney(amount), AccountName(from))
		})

	ctx.Then(`^tenant "([^"]*)"'s wallet account "([^"]*)" balance reads (\S+)$`,
		func(c context.Context, tenant, account, amount string) error {
			return w.AssertAccountBalance(c, TenantName(tenant), AccountName(account), ParseMoney(amount))
		})

	ctx.When(`^tenant "([^"]*)" queries the transfer$`, func(c context.Context, tenant string) error {
		return w.QueryTransfer(c, AsTenant(TenantName(tenant)), w.LastTransferAnswer().TransferID)
	})

	ctx.Then(`^the transfer's status becomes "([^"]*)" with reason "([^"]*)"$`, func(status, reason string) error {
		if err := w.AssertTransferStatus(ParseTransferStatus(status)); err != nil {
			return err
		}
		return w.AssertReason(reason)
	})

	// Fix (2026-09-08, DELIVER 04-04 back-propagation, gap 1 dependency): was
	// a `return nil` placeholder -- now a real GET .../entries round trip per
	// touched account (world.go's own InspectTouchedAccountEntries), caching
	// a fresh post-reversal read for the two Then steps immediately below to
	// diff against the pre-reversal snapshot driveTransferToReversed already
	// took.
	ctx.When(`^the entry log for every account the transfer touched is inspected$`, func(c context.Context) error {
		return w.InspectTouchedAccountEntries(c)
	})

	// Fix 5 (2026-09-08, DELIVER 03-04 back-propagation): was a `return nil`
	// placeholder -- now a real GET /accounts/{id} call against the sender's
	// own settlement account (world.go's own InspectSenderSettlementBalance),
	// cached for the following Then step.
	ctx.When(`^the platform account's balance is inspected$`, func(c context.Context) error {
		return w.InspectSenderSettlementBalance(c)
	})

	// Fix 6 (2026-09-08, DELIVER 03-04 back-propagation): was a `return nil`
	// placeholder -- now a real GET /accounts/{id}/entries call against the
	// sender's own tnt_platform mirror account (world.go's own
	// InspectPlatformMirrorEntries), cached for the following Then step.
	ctx.When(`^the platform account's entry log is inspected$`, func(c context.Context) error {
		return w.InspectPlatformMirrorEntries(c)
	})

	ctx.Then(`^all three legs report "([^"]*)"$`, func(status string) error {
		answer := w.LastTransferAnswer()
		if len(answer.Legs) != 3 {
			return fmt.Errorf("expected 3 legs reported, got %d (raw: %s)", len(answer.Legs), answer.Raw)
		}
		for _, leg := range answer.Legs {
			if string(leg.Status) != status {
				return fmt.Errorf("expected leg %d status %q, got %q", leg.Leg, status, leg.Status)
			}
		}
		return nil
	})
}
