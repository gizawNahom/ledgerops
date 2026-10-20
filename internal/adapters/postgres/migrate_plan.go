package postgres

import (
	"regexp"

	"ledgerops/internal/support"
)

// SCAFFOLD: true -- RED stubs created by DISTILL (Mandate 7); signatures per
// DDD-42 / ADR-022. DELIVER implements; the acceptance suite drives migration
// through the `ledgerops migrate` binary, and unit tests drive these directly.

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

// PlanMigration is pure: facts and config in, refusal or ordered plan out.
func PlanMigration(facts Facts, config MigrateConfig) Plan {
	support.Scaffold("PlanMigration")
	return Plan{}
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
