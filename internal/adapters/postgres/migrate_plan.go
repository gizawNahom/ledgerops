package postgres

import (
	"regexp"

	"ledgerops/internal/support"
)

// Facts is what FactReader and PreflightProber gather before any change.
type Facts struct {
	TargetDatabase       string
	HasLedgerSchema      bool
	DirtyVersion         *uint
	RoleExists           bool
	RoleOwnerMarker      string
	OwnerDatabaseExists  bool
	CanCreateRole        bool
	HasAdminOptionOnRole bool
	OwnsDatabaseOrGrant  bool
	CanCreateOnPublic    bool
	OwnsLedgerTables     bool
	PendingVersions      []uint
}

// MigrateConfig is the parsed, validated migrate request.
type MigrateConfig struct {
	PrivilegedDSN support.Secret
	AppPassword   support.Secret
}

// Step is one ordered action of a Plan.
type Step string

const (
	ClaimRole          Step = "claim-role"
	ApplySchema        Step = "apply-schema"
	SetAppRolePassword Step = "set-app-role-password"
)

// Plan is an ordered list of steps, or a refusal.
type Plan struct {
	Steps     []Step
	Refusal   string
	Privilege string
	NoChanges bool
}

// Refused reports whether the plan is a refusal rather than a list of steps.
func (plan Plan) Refused() bool { return plan.Refusal != "" }

// Privilege names as they appear in a refusal.
const (
	privilegeConnect    = "CONNECT"
	privilegeCreate     = "CREATE on schema public"
	privilegeCreateRole = "CREATEROLE"
	privilegeAdminOpt   = "ADMIN OPTION on ledgerops_app"
	privilegeOwner      = "table owner"
)

// noChangesSuffix closes every preflight refusal: nothing was applied.
const noChangesSuffix = "; no changes applied"

// PlanMigration is pure: facts and config in, refusal or ordered plan out.
// Every check runs before any schema change (ADR-022), so a gap exits naming
// the privilege and saying nothing was changed. The cluster rule and the
// foreign-table-collision rule join this list in step 02-01.
func PlanMigration(facts Facts, config MigrateConfig) Plan {
	for _, check := range preflightChecks {
		if refusal, refused := check(facts); refused {
			return refusal
		}
	}
	return Plan{Steps: []Step{ClaimRole, ApplySchema, SetAppRolePassword}}
}

type preflightCheck func(Facts) (Plan, bool)

var preflightChecks = []preflightCheck{
	requireConnectRight,
	requireCreateOnPublic,
	requireCreateRoleWhenRoleAbsent,
	requireAdminOptionWhenRoleExists,
	requireOwnershipOfTablesToAlter,
}

func refuseMissing(privilege, because string) (Plan, bool) {
	return Plan{
		Refusal:   "the privileged role lacks " + privilege + ": " + because + noChangesSuffix,
		Privilege: privilege,
		NoChanges: true,
	}, true
}

func requireConnectRight(facts Facts) (Plan, bool) {
	if facts.OwnsDatabaseOrGrant {
		return Plan{}, false
	}
	return refuseMissing(privilegeConnect,
		"it neither owns database "+facts.TargetDatabase+" nor may grant CONNECT on it")
}

func requireCreateOnPublic(facts Facts) (Plan, bool) {
	if facts.CanCreateOnPublic {
		return Plan{}, false
	}
	return refuseMissing(privilegeCreate, "migrations create objects in the public schema")
}

func requireCreateRoleWhenRoleAbsent(facts Facts) (Plan, bool) {
	if facts.RoleExists || facts.CanCreateRole {
		return Plan{}, false
	}
	return refuseMissing(privilegeCreateRole, "ledgerops_app does not exist and must be created")
}

func requireAdminOptionWhenRoleExists(facts Facts) (Plan, bool) {
	if !facts.RoleExists || facts.HasAdminOptionOnRole {
		return Plan{}, false
	}
	return refuseMissing(privilegeAdminOpt, "the app role password must be set on the existing ledgerops_app")
}

func requireOwnershipOfTablesToAlter(facts Facts) (Plan, bool) {
	if len(facts.PendingVersions) == 0 || !facts.HasLedgerSchema || facts.OwnsLedgerTables {
		return Plan{}, false
	}
	return refuseMissing(privilegeOwner, "it is not the owner of a ledger table the pending migrations alter")
}

// FactReader gathers facts with read-only catalog queries.
type FactReader interface {
	ReadFacts(target string) (Facts, error)
}

// PreflightProber runs write-capable probes inside a transaction that is
// always rolled back.
type PreflightProber interface {
	Probe(target string) (Facts, error)
}

// SchemaMigrator applies the embedded SQL to head and detects dirty state.
type SchemaMigrator interface {
	ApplyPending() error
}

// RolePasswordSetter accepts a finished SCRAM verifier only, never plaintext.
type RolePasswordSetter interface {
	SetVerifier(verifier string) error
}

// RedactDSN removes the password component from any text derived from a DSN:
// URL userinfo and key=value (or quoted) password settings.
func RedactDSN(text string) string {
	text = urlPasswordPattern.ReplaceAllString(text, "${1}"+support.RedactionMarker+"@")
	return keyValuePasswordPattern.ReplaceAllString(text, "${1}"+support.RedactionMarker)
}

var (
	urlPasswordPattern      = regexp.MustCompile(`(://[^:/@\s]*:)[^@\s]*@`)
	keyValuePasswordPattern = regexp.MustCompile(`(?i)(password\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)
)
