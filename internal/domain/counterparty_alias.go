package domain

// CounterpartyAlias is the aggregate root recording a tenant-scoped name
// that resolves to another tenant's (tenant_id, account_id) — its own
// aggregate, not a child of TenantLink or Tenant (brief.md § Inter-tenant
// transfer, "Vernon's-rules justification").
//
// Full observable state is exactly {tenant_id, alias, tenant_link_id,
// target_tenant_id, target_account_id} — no child entities. Identity is
// the composite (tenant_id, alias): alias uniqueness is scoped to the
// owning tenant's own namespace, never global — two different tenants may
// register the identical alias string with zero collision, because they
// are two different aggregate instances under two different identities.
//
// Immutable with unexported fields, like TenantLink and Account (DDD-15):
// there is no mutating method, so a CounterpartyAlias missing a field is
// never produced.
type CounterpartyAlias struct {
	tenantID        string
	alias           string
	tenantLinkID    string
	targetTenantID  string
	targetAccountID string
}

// TenantID exposes the owning/registering tenant.
func (c CounterpartyAlias) TenantID() string {
	return c.tenantID
}

// Alias exposes the name registered within the owning tenant's namespace.
func (c CounterpartyAlias) Alias() string {
	return c.alias
}

// TenantLinkID exposes the standing authorization this alias was
// registered against — held by id only, never by embedding the TenantLink
// itself (deps note, step 02-02).
func (c CounterpartyAlias) TenantLinkID() string {
	return c.tenantLinkID
}

// TargetTenantID and TargetAccountID expose what this alias resolves to.
func (c CounterpartyAlias) TargetTenantID() string {
	return c.targetTenantID
}

func (c CounterpartyAlias) TargetAccountID() string {
	return c.targetAccountID
}

// RegisterCounterpartyAlias is the pure decision behind registering a
// tenant-scoped counterparty alias (I11, second half, first enforcement
// point). Declared delta: exactly one new {tenant_id, alias,
// tenant_link_id, target_tenant_id, target_account_id} record comes into
// existence — no TenantLink, Account, or Transaction changes
// (brief.md § Inter-tenant transfer, "Complement equality").
//
// The application-layer caller performs the ByID read that produces
// tenantLinkSnapshot and tenantLinkFound (mirroring
// TenantLinkRepository.ByID's own (value, bool, error) shape) — this
// function performs no I/O of its own, exactly like OpenAccount's
// alreadyOpen courtesy check (internal/domain/post.go).
//
// Refuses tenant_link_not_found (existing member, reused from the RED
// scaffold in violation.go) if the referenced link was not found at all,
// or was found but is no longer active. Nothing distinguishes "never
// authorized" from "revoked" here — both collapse to the same refusal,
// since the caller supplied the link reference directly and naming it
// precisely reveals nothing it did not already know it was asking about.
//
// The (tenant_id, alias) composite-identity uniqueness check is
// deliberately NOT enforced here: brief.md's exhaustive new-refusal-
// taxonomy tables for inter-tenant-transfer name exactly three new
// domain.ViolationKind members for this feature (tenant_link_already_exists,
// tenant_link_not_found, counterparty_not_found) and no fourth
// "alias already registered" member. This step's boundary rules also
// forbid adding to violation.go. That uniqueness is therefore left to the
// application layer's read-then-refuse courtesy check backed by the
// counterparty_aliases table's PRIMARY KEY (tenant_id, alias) constraint
// (migration 02-01) as the final backstop under concurrency — flagged for
// confirmation, not guessed.
func RegisterCounterpartyAlias(
	tenantID string,
	alias string,
	tenantLinkSnapshot TenantLink,
	tenantLinkFound bool,
	targetTenantID string,
	targetAccountID string,
) (CounterpartyAlias, error) {
	if !tenantLinkFound || tenantLinkSnapshot.Status() != TenantLinkActive {
		return CounterpartyAlias{}, NewTenantLinkNotFound()
	}

	return CounterpartyAlias{
		tenantID:        tenantID,
		alias:           alias,
		tenantLinkID:    tenantLinkSnapshot.LinkID(),
		targetTenantID:  targetTenantID,
		targetAccountID: targetAccountID,
	}, nil
}

// ResolveCounterparty is the pure decision behind resolving a registered
// alias to the (target_tenant_id, target_account_id) pair a coordinator
// hands to leg 1's Post call (I11, second half, second enforcement point).
// It is a pure decision function, not a mutating command — it mutates
// nothing; its complement is the entire state (a pure read).
//
// The application-layer caller performs the ByTenantAndAlias read that
// produces aliasSnapshot/aliasFound, and — only if an alias was found —
// the ByID read over its tenant_link_id that produces
// tenantLinkSnapshot/tenantLinkFound.
//
// Refuses counterparty_not_found (existing member, reused deliberately)
// if the alias is absent, or names an owning tenant/alias pair that does
// not match the caller's own lookup, or if the tenant-link snapshot it
// names is no longer active. Collapsing "alias never existed" and "alias
// existed but its link died" into the single counterparty_not_found
// refusal — rather than surfacing tenant_link_not_found here — is a
// deliberate info-leak decision: distinguishing the two would hand a
// caller a link-existence oracle, the same class of leak account_not_found
// already closes for I8.
func ResolveCounterparty(
	tenantID string,
	alias string,
	aliasSnapshot CounterpartyAlias,
	aliasFound bool,
	tenantLinkSnapshot TenantLink,
	tenantLinkFound bool,
) (string, string, error) {
	if !aliasFound || aliasSnapshot.tenantID != tenantID || aliasSnapshot.alias != alias {
		return "", "", NewCounterpartyNotFound()
	}
	if !tenantLinkFound || tenantLinkSnapshot.Status() != TenantLinkActive {
		return "", "", NewCounterpartyNotFound()
	}

	return aliasSnapshot.targetTenantID, aliasSnapshot.targetAccountID, nil
}
