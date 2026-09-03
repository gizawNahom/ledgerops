package domain

// Tenant is the aggregate root that scopes every Account and Transaction into
// an isolated ledger owner (multitenancy). It is a new aggregate root, not a
// value object and not a new bounded context: its identity is tenant_id, and
// that identity persists across the aggregate's one lifecycle event
// (provisioning). Account and Transaction reference a Tenant by tenant_id --
// a Tenant never embeds them (Vernon rule 3).
//
// Immutable with unexported fields, like Account (DDD-15): there is no
// mutating method, so a Tenant holding an illegal name is never produced.
type Tenant struct {
	tenantID   string
	name       string
	credential string
}

// NewTenant is the smart constructor. Every field is root-only and
// value-typed (Vernon rule 2) -- name and credential -- and no path
// constructs a Tenant without a tenant_id: it is the first, required
// parameter, exactly like Account.id.
//
// credential is the tenant_key: opaque to the domain beyond "exists, is
// distinct per tenant". Hashing and verification mechanics belong to the
// HTTP/postgres adapters, not here.
func NewTenant(tenantID, name, credential string) (Tenant, error) {
	return Tenant{tenantID: tenantID, name: name, credential: credential}, nil
}

// TenantID exposes the aggregate identifier.
func (t Tenant) TenantID() string {
	return t.tenantID
}

// Name exposes the tenant's display name, whose uniqueness I10 protects.
func (t Tenant) Name() string {
	return t.name
}

// Credential exposes the opaque tenant_key.
func (t Tenant) Credential() string {
	return t.credential
}

// ProvisionTenant is the pure decision behind provisioning a tenant, and
// enforces I10 (tenant-name uniqueness) at construction -- mirroring
// OpenAccount's enforcement of account-id uniqueness (DDD-18) one aggregate
// level up, structurally identical on purpose.
//
// The application-layer caller performs the read that produces
// nameAlreadyTaken -- a lookup against the store -- and this function
// performs no I/O of its own; it is the courtesy check the domain owns,
// ahead of the unique constraint that is the final backstop under
// concurrency. A name already bound is refused rather than treated as a
// retry: nothing here can tell a genuine retry apart from a name collision
// between two independent callers, and guessing "retry" would hand the
// second caller a tenant somebody else provisioned.
func ProvisionTenant(tenantID, name, credential string, nameAlreadyTaken bool) (Tenant, error) {
	if nameAlreadyTaken {
		return Tenant{}, NewTenantAlreadyExists(name)
	}
	return NewTenant(tenantID, name, credential)
}
