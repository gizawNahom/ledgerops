// Package containerimagerelease holds the acceptance suite for the
// container-image-release feature (US-1a .. US-8).
//
// It is its own package, following tests/acceptance/multitenancy: a godog
// suite driven from TestMain, a World composition root, and this file as the
// Mandate-12 domain types module. Every domain noun that appears in a
// .feature file is expressed here once. Step bodies coerce captured text into
// these types and delegate to a World method; no step branches on a literal.
//
// The driving ports are the `ledgerops` binary (serve | migrate | version) run
// as a subprocess or as the built image, the pure internal/release classifier,
// and parsed-YAML views of the release pipeline (policy rows added 2026-10-19).
//
// Output wording is "illustrative" upstream, so every token the suite asserts
// on lives in OutputToken below: if DELIVER words a message differently, that
// is the one place to change.
package containerimagerelease

import (
	"fmt"
	"strings"
)

// ---- fixture secrets (sentinels; never real credentials) -------------------

const (
	DemoPassword     = "app-secret"
	ChosenPassword   = "T7!kq@9x%Zm"
	FilePassword     = "Zr4#mV8$wQ1"
	RotatedPassword  = "Lm2&pT6@sD9"
	WrongGuess       = "wrong-guess"
	OperatorKeyValue = "op-9f3b71c2"
	DemoTenantToken  = "demo-tenant-key"
)

// Fixed clock-free names used by the harness for the privileged role.
const (
	privilegedPassword = "admin-secret"
	legacyDatabase     = "ledgerops"
	legacyPrivileged   = "ledgerops_migrate"
	legacyPrivPassword = "migrate-secret"
	appRole            = "ledgerops_app"
)

// DatabaseName is a Postgres database name ("kestrel_ledger").
type DatabaseName string

// RoleName is a Postgres role name ("kestrel_admin", "ledgerops_app").
type RoleName string

// ---- commands and environment ---------------------------------------------

// Command is a subcommand of the single ledgerops binary (ADR-018).
type Command string

const (
	CommandMigrate Command = "migrate"
	CommandServe   Command = "serve"
	CommandVersion Command = "version"
)

// Variable is an environment variable of the ledgerops contract (ADR-018).
type Variable string

const (
	VarMigrateDSN      Variable = "LEDGEROPS_MIGRATE_DSN"
	VarAppPassword     Variable = "LEDGEROPS_APP_PASSWORD"
	VarAppPasswordFile Variable = "LEDGEROPS_APP_PASSWORD_FILE"
	VarAppDSN          Variable = "LEDGEROPS_APP_DSN"
	VarOperatorKey     Variable = "LEDGEROPS_OPERATOR_KEY"
	VarDemoTenantKey   Variable = "LEDGEROPS_DEMO_TENANT_KEY"
	VarAddr            Variable = "LEDGEROPS_ADDR"
)

// ParseVariable coerces the Gherkin phrasing of a variable.
func ParseVariable(text string) Variable {
	switch strings.TrimSpace(text) {
	case "the operator key", string(VarOperatorKey):
		return VarOperatorKey
	case "the app connection setting", string(VarAppDSN):
		return VarAppDSN
	case "the demo tenant key", string(VarDemoTenantKey):
		return VarDemoTenantKey
	case "the app password", string(VarAppPassword):
		return VarAppPassword
	}
	panic(fmt.Sprintf("unknown variable phrasing %q — see ParseVariable", text))
}

// ValueState is how a setting is present in the environment.
type ValueState string

const (
	Unset          ValueState = "unset"
	EmptyValue     ValueState = "empty"
	WhitespaceOnly ValueState = "whitespace-only"
	HasValue       ValueState = "set"
)

// ParseValueState coerces the Gherkin phrasing.
func ParseValueState(text string) ValueState {
	switch strings.TrimSpace(text) {
	case "set":
		return HasValue
	case "unset", "not set":
		return Unset
	case "empty", "set to an empty value":
		return EmptyValue
	case "whitespace-only", "set to whitespace only":
		return WhitespaceOnly
	}
	panic(fmt.Sprintf("unknown value state %q — see ParseValueState", text))
}

// Literal is the environment value a ValueState stands for ("" for Unset is
// handled by the caller: the variable is simply absent).
func (s ValueState) Literal() string {
	switch s {
	case EmptyValue:
		return ""
	case WhitespaceOnly:
		return "   "
	}
	return ""
}

// LineEnding is what ends a password file.
type LineEnding string

const (
	LineFeed       LineEnding = "\n"
	CarriageReturn LineEnding = "\r\n"
	NoLineEnding   LineEnding = ""
)

// ParseLineEnding coerces the Gherkin phrasing.
func ParseLineEnding(text string) LineEnding {
	switch strings.TrimSpace(text) {
	case "a line feed":
		return LineFeed
	case "a carriage return and line feed":
		return CarriageReturn
	case "no line ending":
		return NoLineEnding
	}
	panic(fmt.Sprintf("unknown line ending %q — see ParseLineEnding", text))
}

// SecretFileAccess is who can read a mounted secret file (environment
// nonroot-secret-files).
type SecretFileAccess string

const (
	WorldReadable SecretFileAccess = "world-readable"
	OwnerOnly     SecretFileAccess = "owner-only"
)

// ---- failure situations (outline parameters) -------------------------------

// FailureSituation is a way a migrate run can fail, for the "output never
// contains the password" outline (sentinel assertion on every failure path).
type FailureSituation string

const (
	NoRoleCreationRight FailureSituation = "the privileged role cannot create roles"
	DirtyPreviousRun    FailureSituation = "a previous run left an unfinished migration"
	DatabaseUnreachable FailureSituation = "the database cannot be reached"
	ConflictingSources  FailureSituation = "both password sources are set"
	SecondDatabase      FailureSituation = "the cluster already holds another ledgerops database"
)

// ParseFailureSituation coerces the Gherkin phrasing.
func ParseFailureSituation(text string) FailureSituation {
	f := FailureSituation(strings.TrimSpace(text))
	switch f {
	case NoRoleCreationRight, DirtyPreviousRun, DatabaseUnreachable, ConflictingSources, SecondDatabase:
		return f
	}
	panic(fmt.Sprintf("unknown failure situation %q — see ParseFailureSituation", text))
}

// PrivilegeGap is one missing privilege of the supported minimum (ADR-022).
type PrivilegeGap string

const (
	MissingCreateRole       PrivilegeGap = "cannot create roles"
	MissingAdminOption      PrivilegeGap = "has no admin option on the app role"
	NotDatabaseOwner        PrivilegeGap = "neither owns the database nor may grant connect on it"
	MissingCreateOnPublic   PrivilegeGap = "cannot create objects in the public schema"
	TableOwnedByAnotherRole PrivilegeGap = "does not own a ledger table that must be altered"
)

// ParsePrivilegeGap coerces the Gherkin phrasing.
func ParsePrivilegeGap(text string) PrivilegeGap {
	g := PrivilegeGap(strings.TrimSpace(text))
	switch g {
	case MissingCreateRole, MissingAdminOption, NotDatabaseOwner, MissingCreateOnPublic, TableOwnedByAnotherRole:
		return g
	}
	panic(fmt.Sprintf("unknown privilege gap %q — see ParsePrivilegeGap", text))
}

// NamedBy lists the tokens the refusal must contain to "name the privilege".
func (g PrivilegeGap) NamedBy() []string {
	switch g {
	case MissingCreateRole:
		return []string{"CREATEROLE"}
	case MissingAdminOption:
		return []string{"ADMIN OPTION", appRole}
	case NotDatabaseOwner:
		return []string{"CONNECT"}
	case MissingCreateOnPublic:
		return []string{"CREATE", "public"}
	case TableOwnedByAnotherRole:
		return []string{"owner"}
	}
	return nil
}

// NeedsExistingRole is true when the gap can only show on a database whose
// app role already exists (an upgrade).
func (g PrivilegeGap) NeedsExistingRole() bool {
	return g == MissingAdminOption || g == TableOwnedByAnotherRole
}

// PasswordForm is a Unicode form of the same typed password (DDD-47).
type PasswordForm string

const (
	Composed   PasswordForm = "composed"
	Decomposed PasswordForm = "decomposed"
	Fullwidth  PasswordForm = "fullwidth"
)

// ParsePasswordForm coerces the Gherkin phrasing.
func ParsePasswordForm(text string) PasswordForm {
	f := PasswordForm(strings.TrimSpace(text))
	switch f {
	case Composed, Decomposed, Fullwidth:
		return f
	}
	panic(fmt.Sprintf("unknown password form %q — see ParsePasswordForm", text))
}

// Render gives the concrete password text of a form. All three forms are the
// same password after SASLprep (NFKC): "Zoë-pass-9X" / "Zoë-pass-9X" /
// fullwidth "Ｚoë-pass-9X".
func (f PasswordForm) Render() string {
	switch f {
	case Composed:
		return "Zoë-pass-9X"
	case Decomposed:
		return "Zoë-pass-9X"
	case Fullwidth:
		return "Ｚoë-pass-9X"
	}
	panic("unknown form")
}

// KillPoint is where a migration run is stopped.
type KillPoint string

const (
	BetweenFiles KillPoint = "between two migration files"
	DuringAFile  KillPoint = "during a migration file"
	BeforeTheSet KillPoint = "before the password step"
)

// ParseKillPoint coerces the Gherkin phrasing.
func ParseKillPoint(text string) KillPoint {
	k := KillPoint(strings.TrimSpace(text))
	switch k {
	case BetweenFiles, DuringAFile, BeforeTheSet:
		return k
	}
	panic(fmt.Sprintf("unknown kill point %q — see ParseKillPoint", text))
}

// SchemaVersion is a migration version.
type SchemaVersion int

// ---- cluster shapes ---------------------------------------------------------

// OwnerState is the condition of the database that owns the app role.
type OwnerState string

const (
	OwnerPresent   OwnerState = "present"
	OwnerDropped   OwnerState = "dropped"
	ClaimAbandoned OwnerState = "abandoned"
)

// ---- console bundle ---------------------------------------------------------

// ConsoleBundle is the state of the console bundle at image build time.
type ConsoleBundle string

const (
	BundleAbsent  ConsoleBundle = "absent"
	BundleEmpty   ConsoleBundle = "empty"
	BundleNoIndex ConsoleBundle = "without an index page"
	BundlePresent ConsoleBundle = "present"
)

// ParseConsoleBundle coerces the Gherkin phrasing.
func ParseConsoleBundle(text string) ConsoleBundle {
	b := ConsoleBundle(strings.TrimSpace(text))
	switch b {
	case BundleAbsent, BundleEmpty, BundleNoIndex, BundlePresent:
		return b
	}
	panic(fmt.Sprintf("unknown console bundle state %q — see ParseConsoleBundle", text))
}

// ---- release ---------------------------------------------------------------

// ReleaseTag is a git tag ("v0.2.0").
type ReleaseTag string

// ImageTag is a registry tag ("0.2.0", "latest", "sha256-ab12.sig").
type ImageTag string

// WorkflowJob is a job of release.yml (OPS-14).
type WorkflowJob string

const (
	JobClassify      WorkflowJob = "classify"
	JobCI            WorkflowJob = "ci"
	JobBuild         WorkflowJob = "build"
	JobSmoke         WorkflowJob = "smoke"
	JobSign          WorkflowJob = "sign"
	JobAnonProbe     WorkflowJob = "anon-probe"
	JobPromote       WorkflowJob = "promote"
	JobGitHubRelease WorkflowJob = "github-release"
	JobQuickstart    WorkflowJob = "quickstart"
)

// ParseWorkflowJob coerces the Gherkin phrasing.
func ParseWorkflowJob(text string) WorkflowJob {
	j := WorkflowJob(strings.TrimSpace(text))
	switch j {
	case JobClassify, JobCI, JobBuild, JobSmoke, JobSign, JobAnonProbe, JobPromote, JobGitHubRelease, JobQuickstart:
		return j
	}
	panic(fmt.Sprintf("unknown workflow job %q — see ParseWorkflowJob", text))
}

// Paths of the repository artefacts the structural scenarios read.
const (
	releaseWorkflowPath = ".github/workflows/release.yml"
	ciWorkflowPath      = ".github/workflows/ci.yml"
	dockerfilePath      = "Dockerfile"
	composePath         = "docker-compose.yml"
	loadtestComposePath = "docker-compose.loadtest.yml"
	exampleComposePath  = "examples/published-image/docker-compose.yml"
	exampleReadmePath   = "examples/published-image/README.md"
	quickstartPath      = "docs/quickstart-published-image.md"
	readmePath          = "README.md"
)

// Promote-job contract (upstream-issues U-8): the job's run steps read their
// inputs from exactly these environment variables.
const (
	EnvImageRepo         = "IMAGE_REPO"
	EnvImageDigest       = "IMAGE_DIGEST"
	EnvPlanVersion       = "PLAN_VERSION"
	EnvPlanTags          = "PLAN_TAGS"
	EnvPlanMoveLatest    = "PLAN_MOVE_LATEST"
	EnvPlanHighestStable = "PLAN_HIGHEST_STABLE"
)

// OutputToken is every phrase the suite looks for in program output. Upstream
// wording is illustrative (upstream-issues U-5); change a token here, not in
// a step.
type OutputToken string

const (
	TokenSchemaAt          OutputToken = "schema at version"
	TokenPasswordSet       OutputToken = "app role password set"
	TokenPasswordUpdated   OutputToken = "app role password updated"
	TokenNoPending         OutputToken = "no pending migrations"
	TokenNoChangesApplied  OutputToken = "no changes applied"
	TokenPasswordUnchanged OutputToken = "password was not changed"
	TokenConfigInvalid     OutputToken = "config invalid"
	TokenDirty             OutputToken = "dirty"
	TokenDropRole          OutputToken = "DROP ROLE ledgerops_app"
	TokenAuthFailed        OutputToken = "authentication failed"
	TokenNotReadable       OutputToken = "not readable"
	TokenRoleExists        OutputToken = "ledgerops_app"
	TokenNotAVersionTag    OutputToken = "not a version tag"
	TokenMigrateStep       OutputToken = "migrate"
	TokenPlanStale         OutputToken = "plan stale, re-run all jobs"
)

// RefusalToken maps the business phrasing used in a .feature file to the
// token that appears in program output (one place to change if wording moves).
func RefusalToken(phrase string) string {
	switch phrase {
	case "unfinished migration":
		return string(TokenDirty)
	}
	return phrase
}
