package domain

// TenantLinkStatus is the closed lifecycle taxonomy for a TenantLink (I11).
// Go has no sum types, so — like ViolationKind — the closed set is a string
// discriminant. There is no third status: a link is authorized active and,
// terminally, revoked. Nothing re-activates a revoked link (brief.md §
// Inter-tenant transfer, "Terminal: nothing re-activates a revoked link").
type TenantLinkStatus string

const (
	// TenantLinkActive names a standing authorization currently in force.
	TenantLinkActive TenantLinkStatus = "active"
	// TenantLinkRevoked names a standing authorization no longer in force.
	// Terminal — no domain path transitions a revoked link back to active.
	TenantLinkRevoked TenantLinkStatus = "revoked"
)

// TenantLink is the aggregate root recording a standing, bidirectional
// authorization between two tenants (ADR-014/ADR-015) — its own aggregate,
// not a value object and not a child of Tenant (brief.md § Inter-tenant
// transfer, Vernon's-rules justification).
//
// Full observable state is exactly {link_id, tenant_a, tenant_b, status} —
// no child entities. tenant_a/tenant_b hold the pair canonicalized
// (tenant_a < tenant_b lexicographically) so the same unordered pair always
// occupies the same logical slot regardless of the order a caller named it
// in (I11).
//
// Immutable with unexported fields, like Account and Tenant (DDD-15): there
// is no mutating method, so a TenantLink holding an illegal pair or an
// empty tenant id is never produced — I11's own "no domain path constructs
// a TenantLink without both tenant ids populated" is enforced structurally,
// not by a runtime check scattered elsewhere.
type TenantLink struct {
	linkID  string
	tenantA string
	tenantB string
	status  TenantLinkStatus
}

// LinkID exposes the aggregate identifier.
func (l TenantLink) LinkID() string {
	return l.linkID
}

// TenantA and TenantB expose the canonicalized pair — TenantA is always the
// lexicographically smaller of the two tenant ids.
func (l TenantLink) TenantA() string {
	return l.tenantA
}

func (l TenantLink) TenantB() string {
	return l.tenantB
}

// Status exposes the lifecycle discriminant.
func (l TenantLink) Status() TenantLinkStatus {
	return l.status
}

// canonicalizePair sorts an unordered tenant pair lexicographically so
// AuthorizeTenantPair(b, a) checks the identical logical slot as an
// existing AuthorizeTenantPair(a, b) — canonicalization happens before any
// comparison against a snapshot, which is what makes it load-bearing rather
// than cosmetic (brief.md § Inter-tenant transfer).
func canonicalizePair(tenantA, tenantB string) (string, string) {
	if tenantA > tenantB {
		return tenantB, tenantA
	}
	return tenantA, tenantB
}

// AuthorizeTenantPair is the pure decision behind granting a standing
// authorization between two tenants (I11, first half). Declared delta:
// exactly one new {link_id, tenant_a, tenant_b, status: active} triple
// comes into existence; every other link in existingLinks is unchanged
// (complement equality).
//
// The pair is canonicalized before ever being compared against
// existingLinks, so a caller cannot evade tenant_link_already_exists by
// reversing argument order.
//
// The application-layer caller performs the reads that produce
// existingLinks (a snapshot of already-authorized pairs) and knownTenants
// (which tenant ids are currently provisioned) — this function performs no
// I/O of its own, mirroring OpenAccount's alreadyOpen and ProvisionTenant's
// nameAlreadyTaken courtesy-check division of labor. linkID, like
// transactionID for Post and tenantID for ProvisionTenant, is minted by the
// caller (IDGenerator port) and simply carried through — the domain core
// generates no identifiers.
//
// Refuses tenant_not_found (existing member, reused unchanged) if either
// named tenant is absent from knownTenants. Refuses
// tenant_link_already_exists (new member, reused from the RED scaffold in
// violation.go) if an active link already exists for the canonicalized
// pair in existingLinks.
func AuthorizeTenantPair(linkID, tenantA, tenantB string, existingLinks []TenantLink, knownTenants map[string]bool) (TenantLink, error) {
	canonicalA, canonicalB := canonicalizePair(tenantA, tenantB)

	if !knownTenants[canonicalA] {
		return TenantLink{}, NewTenantNotFound(canonicalA)
	}
	if !knownTenants[canonicalB] {
		return TenantLink{}, NewTenantNotFound(canonicalB)
	}

	for _, existing := range existingLinks {
		if existing.status == TenantLinkActive && existing.tenantA == canonicalA && existing.tenantB == canonicalB {
			return TenantLink{}, NewTenantLinkAlreadyExists()
		}
	}

	return TenantLink{
		linkID:  linkID,
		tenantA: canonicalA,
		tenantB: canonicalB,
		status:  TenantLinkActive,
	}, nil
}

// RevokeTenantLink is the pure decision behind ending a standing
// authorization (I11, terminal transition). Declared delta: status on
// exactly the named linkID transitions active -> revoked; every other link
// in existingLinks is unchanged (complement equality). Nothing re-activates
// a revoked link — a fresh AuthorizeTenantPair call for the same pair mints
// a distinct link_id rather than toggling this one back.
//
// The application-layer caller performs the ByID read that produces
// existingLinks; this function performs no I/O of its own. Refuses
// tenant_link_not_found (existing member, reused from the RED scaffold in
// violation.go) if no link named linkID is present in existingLinks.
func RevokeTenantLink(linkID string, existingLinks []TenantLink) (TenantLink, error) {
	for _, existing := range existingLinks {
		if existing.linkID == linkID {
			return TenantLink{
				linkID:  existing.linkID,
				tenantA: existing.tenantA,
				tenantB: existing.tenantB,
				status:  TenantLinkRevoked,
			}, nil
		}
	}
	return TenantLink{}, NewTenantLinkNotFound()
}
