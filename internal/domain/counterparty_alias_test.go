package domain_test

// Property-based unit tests for the CounterpartyAlias aggregate (step
// 02-02, inter-tenant-transfer). No pre-authored acceptance test targets
// this file in isolation -- it is exercised end-to-end starting at step
// 02-04's HTTP wiring -- so these PBT unit tests ARE this step's own
// RED/GREEN obligation, mirroring tenant_link_test.go's style exactly
// (step 01-02).
//
// Composite-identity collision (alreadyRegistered=true) note: brief.md's
// exhaustive new-refusal-taxonomy tables for inter-tenant-transfer
// (§ Domain Model error table, § Driving ports "New refusal-taxonomy
// members reachable through these ports") enumerate exactly three new
// domain.ViolationKind members for this feature --
// tenant_link_already_exists, tenant_link_not_found, and
// counterparty_not_found -- with no fourth "alias already registered"
// member anywhere in the document. This step's own BOUNDARY_RULES forbid
// touching violation.go, so no new ViolationKind can be minted here in any
// case. Per this step's own escalation guidance ("verify against DESIGN's
// own text before inventing a new violation member; if in doubt, flag it
// rather than guessing"), the composite-identity uniqueness check
// ((tenant_id, alias) collision) is therefore treated as caught only by
// the application layer's read-then-refuse courtesy check backed by the
// `counterparty_aliases` table's PRIMARY KEY (tenant_id, alias) constraint
// (migration 02-01) -- not enforced by a domain-level violation return in
// RegisterCounterpartyAlias. Flagged for confirmation in the step report;
// no test asserts a domain-level "already registered" refusal.

import (
	"errors"
	"testing"

	"ledgerops/internal/domain"
	"pgregory.net/rapid"
)

func genAliasTenantID(t *rapid.T, label string) string {
	return rapid.StringMatching(`tnt_[a-z0-9]{4,8}`).Draw(t, label)
}

func genLinkIDForAlias(t *rapid.T, label string) string {
	return rapid.StringMatching(`lnk_[a-z0-9]{4,8}`).Draw(t, label)
}

func genAliasName(t *rapid.T, label string) string {
	return rapid.StringMatching(`[a-z][a-z0-9-]{2,12}`).Draw(t, label)
}

func genAccountID(t *rapid.T, label string) string {
	return rapid.StringMatching(`acc_[a-z0-9]{4,8}`).Draw(t, label)
}

// activeLink builds an active TenantLink snapshot for the given owning and
// target tenants by driving it through the real AuthorizeTenantPair
// constructor -- never hand-assembled, since TenantLink has no exported
// constructor other than that one (mirrors tenant_link_test.go's own
// convention of only ever producing TenantLink values through the domain
// API).
func activeLink(t *rapid.T, linkID, tenantA, tenantB string) domain.TenantLink {
	link, err := domain.AuthorizeTenantPair(linkID, tenantA, tenantB, nil, map[string]bool{
		tenantA: true,
		tenantB: true,
	})
	if err != nil {
		t.Fatalf("activeLink: AuthorizeTenantPair unexpectedly refused: %v", err)
	}
	return link
}

// TestProperty_RegisterCounterpartyAlias_SucceedsAgainstActiveLink covers
// (a): registration succeeds when the referenced tenant-link snapshot is
// present and active, and the returned aggregate carries exactly the
// identity and target fields the caller supplied.
func TestProperty_RegisterCounterpartyAlias_SucceedsAgainstActiveLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		registered, err := domain.RegisterCounterpartyAlias(owningTenant, alias, link, true, targetTenant, targetAccountID)
		if err != nil {
			t.Fatalf("RegisterCounterpartyAlias unexpectedly refused: %v", err)
		}

		if registered.TenantID() != owningTenant {
			t.Fatalf("TenantID() = %q, want %q", registered.TenantID(), owningTenant)
		}
		if registered.Alias() != alias {
			t.Fatalf("Alias() = %q, want %q", registered.Alias(), alias)
		}
		if registered.TenantLinkID() != linkID {
			t.Fatalf("TenantLinkID() = %q, want %q", registered.TenantLinkID(), linkID)
		}
		if registered.TargetTenantID() != targetTenant {
			t.Fatalf("TargetTenantID() = %q, want %q", registered.TargetTenantID(), targetTenant)
		}
		if registered.TargetAccountID() != targetAccountID {
			t.Fatalf("TargetAccountID() = %q, want %q", registered.TargetAccountID(), targetAccountID)
		}
	})
}

// TestProperty_RegisterCounterpartyAlias_RefusesAbsentLink covers (b):
// registration is refused tenant_link_not_found (existing member, reused)
// when the caller's read found no tenant link at all -- tenantLinkFound is
// false, mirroring OpenAccount's alreadyOpen courtesy-check shape.
func TestProperty_RegisterCounterpartyAlias_RefusesAbsentLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		alias := genAliasName(t, "alias")
		targetTenant := genAliasTenantID(t, "targetTenant")
		targetAccountID := genAccountID(t, "targetAccountID")

		_, err := domain.RegisterCounterpartyAlias(owningTenant, alias, domain.TenantLink{}, false, targetTenant, targetAccountID)

		var violation domain.Violation
		if !errors.As(err, &violation) || violation.Kind() != domain.TenantLinkNotFound {
			t.Fatalf("expected tenant_link_not_found for an absent link, got %v", err)
		}
	})
}

// TestProperty_RegisterCounterpartyAlias_RefusesRevokedLink covers (b),
// second half: a link that was found but has since been revoked is
// refused identically to an absent one -- tenant_link_not_found, not a
// distinct "revoked" refusal.
func TestProperty_RegisterCounterpartyAlias_RefusesRevokedLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)
		revoked, err := domain.RevokeTenantLink(linkID, []domain.TenantLink{link})
		if err != nil {
			t.Fatalf("RevokeTenantLink unexpectedly refused: %v", err)
		}

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		_, err = domain.RegisterCounterpartyAlias(owningTenant, alias, revoked, true, targetTenant, targetAccountID)

		var violation domain.Violation
		if !errors.As(err, &violation) || violation.Kind() != domain.TenantLinkNotFound {
			t.Fatalf("expected tenant_link_not_found for a revoked link, got %v", err)
		}
	})
}

// TestProperty_RegisterCounterpartyAlias_DoesNotMutateSnapshot covers (g):
// RegisterCounterpartyAlias's declared delta is exactly one new alias
// record -- it takes no existing-aliases collection at all (unlike
// AuthorizeTenantPair's existingLinks slice), so complement equality
// reduces to the trivial-but-load-bearing property that calling it never
// mutates the tenant-link snapshot it was handed.
func TestProperty_RegisterCounterpartyAlias_DoesNotMutateSnapshot(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)
		before := link

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		_, err := domain.RegisterCounterpartyAlias(owningTenant, alias, link, true, targetTenant, targetAccountID)
		if err != nil {
			t.Fatalf("RegisterCounterpartyAlias unexpectedly refused: %v", err)
		}

		if link != before {
			t.Fatalf("tenant-link snapshot mutated: before=%+v after=%+v", before, link)
		}
	})
}

// TestProperty_ResolveCounterparty_SucceedsForRegisteredAliasAgainstActiveLink
// covers (d): resolution succeeds and returns (target_tenant_id,
// target_account_id) for a registered alias whose named link is still
// active.
func TestProperty_ResolveCounterparty_SucceedsForRegisteredAliasAgainstActiveLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		registered, err := domain.RegisterCounterpartyAlias(owningTenant, alias, link, true, targetTenant, targetAccountID)
		if err != nil {
			t.Fatalf("RegisterCounterpartyAlias unexpectedly refused: %v", err)
		}

		resolvedTenant, resolvedAccount, err := domain.ResolveCounterparty(owningTenant, alias, registered, true, link, true)
		if err != nil {
			t.Fatalf("ResolveCounterparty unexpectedly refused: %v", err)
		}
		if resolvedTenant != targetTenant {
			t.Fatalf("resolved target tenant = %q, want %q", resolvedTenant, targetTenant)
		}
		if resolvedAccount != targetAccountID {
			t.Fatalf("resolved target account = %q, want %q", resolvedAccount, targetAccountID)
		}
	})
}

// TestProperty_ResolveCounterparty_RefusesAbsentAlias covers (e):
// resolution is refused counterparty_not_found when the caller's read
// found no alias at all.
func TestProperty_ResolveCounterparty_RefusesAbsentAlias(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		alias := genAliasName(t, "alias")

		_, _, err := domain.ResolveCounterparty(owningTenant, alias, domain.CounterpartyAlias{}, false, domain.TenantLink{}, false)

		var violation domain.Violation
		if !errors.As(err, &violation) || violation.Kind() != domain.CounterpartyNotFound {
			t.Fatalf("expected counterparty_not_found for an absent alias, got %v", err)
		}
	})
}

// TestProperty_ResolveCounterparty_CollapsesDeadLinkIntoCounterpartyNotFound
// covers (f): the info-leak discipline. When the alias exists but the
// tenant link it names has since gone inactive (revoked or simply not
// found by the caller's ByID read), resolution is refused
// counterparty_not_found -- NOT tenant_link_not_found -- so a caller
// cannot distinguish "alias never existed" from "alias's link died."
func TestProperty_ResolveCounterparty_CollapsesDeadLinkIntoCounterpartyNotFound(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		registered, err := domain.RegisterCounterpartyAlias(owningTenant, alias, link, true, targetTenant, targetAccountID)
		if err != nil {
			t.Fatalf("RegisterCounterpartyAlias unexpectedly refused: %v", err)
		}

		linkNowDead := rapid.SampledFrom([]string{"revoked", "not_found"}).Draw(t, "linkNowDead")
		var deadLinkSnapshot domain.TenantLink
		var deadLinkFound bool
		if linkNowDead == "revoked" {
			revoked, err := domain.RevokeTenantLink(linkID, []domain.TenantLink{link})
			if err != nil {
				t.Fatalf("RevokeTenantLink unexpectedly refused: %v", err)
			}
			deadLinkSnapshot = revoked
			deadLinkFound = true
		} else {
			deadLinkSnapshot = domain.TenantLink{}
			deadLinkFound = false
		}

		_, _, err = domain.ResolveCounterparty(owningTenant, alias, registered, true, deadLinkSnapshot, deadLinkFound)

		var violation domain.Violation
		if !errors.As(err, &violation) || violation.Kind() != domain.CounterpartyNotFound {
			t.Fatalf("expected counterparty_not_found (info-leak discipline), got %v", err)
		}
		if errors.As(err, &violation) && violation.Kind() == domain.TenantLinkNotFound {
			t.Fatalf("info-leak: ResolveCounterparty must never surface tenant_link_not_found")
		}
	})
}

// TestProperty_ResolveCounterparty_NeverMutatesState covers (h):
// ResolveCounterparty is a pure decision function, not a mutating
// command -- its complement is the entire state. Calling it must never
// change the alias or tenant-link snapshots it was handed.
func TestProperty_ResolveCounterparty_NeverMutatesState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owningTenant := genAliasTenantID(t, "owningTenant")
		targetTenant := genAliasTenantID(t, "targetTenant")
		if owningTenant == targetTenant {
			t.Skip("generator drew the same tenant twice -- not a pair")
		}
		linkID := genLinkIDForAlias(t, "linkID")
		link := activeLink(t, linkID, owningTenant, targetTenant)

		alias := genAliasName(t, "alias")
		targetAccountID := genAccountID(t, "targetAccountID")

		registered, err := domain.RegisterCounterpartyAlias(owningTenant, alias, link, true, targetTenant, targetAccountID)
		if err != nil {
			t.Fatalf("RegisterCounterpartyAlias unexpectedly refused: %v", err)
		}

		beforeAlias := registered
		beforeLink := link

		_, _, _ = domain.ResolveCounterparty(owningTenant, alias, registered, true, link, true)

		if registered != beforeAlias {
			t.Fatalf("alias snapshot mutated: before=%+v after=%+v", beforeAlias, registered)
		}
		if link != beforeLink {
			t.Fatalf("tenant-link snapshot mutated: before=%+v after=%+v", beforeLink, link)
		}
	})
}
