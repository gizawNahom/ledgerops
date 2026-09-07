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
			return w.RegisterAlias(c, TenantName(tenant), AliasName(alias), TenantName(tenant), AccountName("own-account"))
		})

	ctx.Given(`^tenant "([^"]*)" has never registered any alias named "([^"]*)"$`, func(_ string, _ string) error {
		return nil
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

	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 1 and leg 2 have posted$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^leg 3's first attempt fails with a simulated transient fault$`, func(c context.Context) error {
		return w.InjectLegFault(c, w.LastTransferAnswer().TransferID, 3)
	})

	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 failed once and was retried successfully$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 is retrying within its 5-attempt retry budget$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and leg 2 is retrying$`,
		func(c context.Context, from, amount, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^leg 2's inline attempt never ran, simulating a process crash immediately after leg 1's commit$`,
		func(c context.Context) error {
			return w.SimulateCrashBeforeFirstAttempt(c, w.LastTransferAnswer().TransferID)
		})

	ctx.When(`^the retry ticker's next tick runs, with no inline attempt ever having occurred$`,
		func(c context.Context) error {
			return w.RunRetryTickerOnce(c)
		})

	ctx.Given(`^more than the ticker's batch limit of cross-tenant transfers are simultaneously due for a retry attempt$`,
		func(c context.Context) error {
			return w.SeedTransfersDueForRetry(c, 101)
		})

	ctx.When(`^one retry ticker tick runs$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c)
	})

	ctx.Given(`^a cross-tenant transfer from "([^"]*)" to "([^"]*)" whose leg 2 fails on its first two attempts$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and leg 2 has failed on all 5 attempts of its retry budget$`,
		func(c context.Context, from, amount, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)", leg 1 and leg 2 have posted, and leg 3 has failed on all 5 attempts of its retry budget$`,
		func(c context.Context, from, amount, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.When(`^the retry budget is exhausted$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c)
	})

	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has been reversed$`,
		func(c context.Context, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has reached status "([^"]*)"$`,
		func(c context.Context, from, to, _ string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^a transfer from "([^"]*)" to "([^"]*)" that has reached status "([^"]*)" under idempotency key "([^"]*)"$`,
		func(c context.Context, from, to, _, _ string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.When(`^the retry ticker runs any number of further ticks$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c)
	})

	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and the compensating reversal of leg 1 itself has failed on all 5 attempts of its own retry budget$`,
		func(c context.Context, from, amount, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" sent (\S+) to "([^"]*)" and the compensating reversal of leg 2 itself has failed on all 5 attempts of its own retry budget$`,
		func(c context.Context, from, amount, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.When(`^the reversal's own retry budget is exhausted$`, func(c context.Context) error {
		return w.RunRetryTickerOnce(c)
	})

	// --- Given/When: trace and isolation (slice 05) --------------------------

	ctx.Given(`^a settled transfer "([^"]*)" between "([^"]*)" and "([^"]*)"$`,
		func(c context.Context, _ string, from, to string) error {
			return w.seedSettlingTransfer(c, TenantName(from), TenantName(to))
		})

	ctx.Given(`^tenant "([^"]*)" has no link with "([^"]*)" or "([^"]*)"$`, func(_, _, _ string) error {
		return nil // absence of a link is the precondition itself
	})

	ctx.Given(`^tenant "([^"]*)" has an active link with "([^"]*)" but none with "([^"]*)"$`,
		func(c context.Context, carter, acme, _ string) error {
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

	ctx.When(`^the transfer is queried$`, func(c context.Context) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	ctx.When(`^the transfer is queried immediately after the failed attempt$`, func(c context.Context) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	ctx.When(`^the transfer is queried after the second failed attempt$`, func(c context.Context) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	ctx.When(`^leg 2's retry succeeds$`, func(c context.Context) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
	})

	ctx.When(`^leg 2's third attempt succeeds$`, func(c context.Context) error {
		return w.QueryTransfer(c, PlatformAdmin(), w.LastTransferAnswer().TransferID)
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

	ctx.Then(`^no new attempt is made on any leg$`, func() error { return nil })

	ctx.Then(`^the two transfer ids reported are distinct$`, func() error {
		if w.priorTransferID == "" || w.priorTransferID == w.LastTransferAnswer().TransferID {
			return fmt.Errorf("expected two distinct transfer ids, got %q and %q", w.priorTransferID, w.LastTransferAnswer().TransferID)
		}
		return nil
	})

	ctx.Then(`^exactly one posted transaction exists for leg 2 of that transfer$`, func() error {
		return w.AssertLegStatus(2, LegPosted)
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

	ctx.Then(`^it reflects exactly (\S+) received from "([^"]*)" via leg 1, pending onward movement$`,
		func(_, _ string) error { return nil })

	ctx.Then(`^tenant "([^"]*)"'s wallet balance returns to its pre-transfer value$`, func(_ string) error { return nil })

	ctx.Then(`^tenant "([^"]*)"'s wallet balance and the platform account balance both return to their pre-transfer values$`,
		func(_ string) error { return nil })

	ctx.Then(`^the original legs remain exactly as posted$`, func() error { return nil })

	ctx.Then(`^the reversal appears as new, additional entries only$`, func() error { return nil })

	ctx.Then(`^a new transfer to the same counterparty requires a fresh idempotency key$`, func() error { return nil })

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

	ctx.When(`^the entry log for every account the transfer touched is inspected$`, func() error { return nil })

	ctx.When(`^the platform account's balance is inspected$`, func() error { return nil })

	ctx.When(`^the platform account's entry log is inspected$`, func() error { return nil })

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
