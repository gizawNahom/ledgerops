package containerimagerelease

import (
	"context"
	"regexp"

	"github.com/cucumber/godog"
)

// Step definitions. Per Mandate-12 every body is one statement: coerce the
// captured text into a domain type and delegate to a World method. No
// branching, no loops, no business logic (mirrors multitenancy's convention).

var quotedText = regexp.MustCompile(`"([^"]*)"`)

// quoted returns every double-quoted string in text, in order.
func quoted(text string) []string {
	var out []string
	for _, m := range quotedText.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func RegisterSteps(ctx *godog.ScenarioContext, w *World) {

	// --- clusters, credentials and fixtures -------------------------------------
	ctx.Step(`^(?:the operator|Priya) has an empty database "([^"]*)" and a privileged role "([^"]*)"$`,
		func(c context.Context, db, role string) error {
			return w.GivenEmptyDatabase(c, DatabaseName(db), RoleName(role))
		})
	ctx.Step(`^the operator has an empty database "([^"]*)" and a privileged role "([^"]*)" on a server that logs every statement$`,
		func(c context.Context, db, role string) error {
			return w.GivenDdlLoggingCluster(c, DatabaseName(db), RoleName(role))
		})
	ctx.Step(`^the database "([^"]*)" was migrated with the app password "([^"]*)"$`,
		func(c context.Context, db, pw string) error { return w.GivenMigratedDatabase(c, DatabaseName(db), pw) })
	ctx.Step(`^a database created by the demo stack at schema version (\d+) with (\d+) entries$`,
		func(c context.Context, v, n int) error { return w.GivenLegacyDatabase(c, SchemaVersion(v), n) })
	ctx.Given(`^the app role of that database accepts "([^"]*)"$`,
		func(c context.Context, pw string) error { return w.ThenAppRoleLogsIn(c, pw) })
	ctx.Step(`^a later migration is made to fail$`,
		func(c context.Context) error { return w.GivenInjectedFailureInLaterMigration(c) })

	ctx.Step(`^the app password is "([^"]*)"$`, func(pw string) error { return w.GivenPassword(pw) })
	ctx.Step(`^the app password is "([^"]*)" and the password file holds "([^"]*)"$`,
		func(inline, file string) error { return w.GivenBothSources(inline, file) })
	ctx.Step(`^the app password is (not set|set to an empty value|set to whitespace only)$`,
		func(state string) error { return w.GivenPasswordSetting(ParseValueState(state)) })
	ctx.Step(`^the app password is typed in its (composed|decomposed|fullwidth) form$`,
		func(form string) error { return w.GivenPasswordForm(ParsePasswordForm(form)) })
	ctx.Step(`^the password file holds "([^"]*)" followed by (.+)$`,
		func(content, ending string) error { return w.GivenPasswordFile(content, ParseLineEnding(ending)) })
	ctx.Step(`^the password file can be read by any user$`, func() error { return w.GivenFileAccess(WorldReadable) })
	ctx.Step(`^the password file can be read only by its owner while the image runs as another user$`,
		func() error { return w.GivenFileAccess(OwnerOnly) })
	ctx.Step(`^the app login uses the password "([^"]*)"$`, func(pw string) error { return w.GivenAppLoginPassword(pw) })

	ctx.Step(`^(the privileged role cannot create roles|a previous run left an unfinished migration|the database cannot be reached|both password sources are set|the cluster already holds another ledgerops database)$`,
		func(c context.Context, situation string) error {
			return w.GivenFailureSituation(c, ParseFailureSituation(situation))
		})
	ctx.When(`^a migration run is killed (between two migration files|during a migration file|before the password step)$`,
		func(c context.Context, point string) error {
			return w.RunMigrateKilledAt(c, ParseKillPoint(point), false)
		})
	ctx.Given(`^a previous run was killed (between two migration files|during a migration file|before the password step)$`,
		func(c context.Context, point string) error {
			return w.RunMigrateKilledAt(c, ParseKillPoint(point), true)
		})
	ctx.Given(`^the database has been prepared with migrate$`, func(c context.Context) error { return w.GivenDatabasePrepared(c) })
	ctx.Given(`^ledgerops is running$`, func(c context.Context) error { return w.GivenLedgerOpsRunning(c) })

	// --- cluster rule ---------------------------------------------------------------
	ctx.Step(`^the cluster also holds an empty database "([^"]*)"$`,
		func(c context.Context, db string) error {
			return w.GivenSecondEmptyDatabaseAfterOwner(c, DatabaseName(db))
		})
	ctx.Step(`^the owning database "([^"]*)" has been dropped$`,
		func(c context.Context, db string) error { return w.GivenOwnerDatabaseDropped(c, DatabaseName(db)) })
	ctx.Step(`^a first migrate of "([^"]*)" was abandoned after claiming the role, and the cluster also holds "([^"]*)"$`,
		func(c context.Context, first, second string) error {
			return w.GivenAbandonedClaim(c, DatabaseName(first), DatabaseName(second))
		})
	ctx.Step(`^two empty databases "([^"]*)" and "([^"]*)" in one cluster$`,
		func(c context.Context, a, b string) error {
			return w.GivenTwoEmptyDatabases(c, DatabaseName(a), DatabaseName(b))
		})
	ctx.Step(`^the operator prepares "([^"]*)" with migrate$`,
		func(c context.Context, db string) error { return w.RunMigrateAgainst(c, DatabaseName(db)) })
	ctx.Step(`^two migrate runs start at the same time, one for each database$`,
		func(c context.Context) error { return w.RunTwoMigratesForTheTrackedDatabases(c) })
	ctx.Step(`^two migrate runs start at the same time, both for "([^"]*)"$`,
		func(c context.Context, db string) error {
			return w.RunTwoMigratesTogether(c, DatabaseName(db), DatabaseName(db))
		})
	ctx.Step(`^exactly one succeeds and the other refuses with the cluster-rule message$`,
		func() error { return w.ThenExactlyOneSucceedsAndTheOtherRefuses() })
	ctx.Step(`^at least one succeeds$`, func(c context.Context) error { return w.ThenAtLeastOneSucceedsAndBothEndConsistent(c) })
	ctx.Step(`^neither database is left in a dirty migration state$`, func(c context.Context) error { return w.ThenNeitherDatabaseIsDirty(c) })
	ctx.Step(`^only one database owns the app role$`, func(c context.Context) error { return w.ThenOnlyOneDatabaseOwnsTheRole(c) })

	// --- privileged-role fixtures -----------------------------------------------------
	ctx.Step(`^a privileged role that holds exactly the supported minimum$`,
		func(c context.Context) error { return w.GivenPrivilegedRoleWithMinimum(c) })
	ctx.Step(`^a privileged role that does not own the database but may grant connect on it$`,
		func(c context.Context) error { return w.GivenPrivilegedRoleThatMayGrantConnectButDoesNotOwn(c) })
	ctx.Step(`^a privileged role that (cannot create roles|has no admin option on the app role|neither owns the database nor may grant connect on it|cannot create objects in the public schema|does not own a ledger table that must be altered)$`,
		func(c context.Context, gap string) error {
			return w.GivenPrivilegedRoleWithGap(c, ParsePrivilegeGap(gap))
		})

	// --- when: migrate -------------------------------------------------------------------
	ctx.Step(`^(?:the operator|Priya) (?:prepares the database|upgrades the database) with migrate$`,
		func(c context.Context) error { return w.RunMigrate(c) })
	ctx.Step(`^the operator runs migrate again$`, func(c context.Context) error { return w.RunMigrate(c) })
	ctx.Step(`^the operator prepares the database with migrate while the demo password is tried continuously$`,
		func(c context.Context) error { return w.RunMigrateWatched(c) })

	// --- then: migrate ---------------------------------------------------------------------
	ctx.Step(`^migrate succeeds and reports schema version (\d+)$`,
		func(v int) error { return w.ThenMigrateSucceeds(SchemaVersion(v)) })
	ctx.Step(`^migrate refuses, naming the unfinished migration (\d+)$`,
		func(v int) error { return w.ThenMigrateRefusesNamingUnfinished(SchemaVersion(v)) })
	ctx.Step(`^migrate refuses, saying the password was not changed$`, func() error { return w.ThenMigrateRefusesSayingPasswordUnchanged() })
	ctx.Step(`^the database stays at version (\d+) with no unfinished migration$`,
		func(c context.Context, v int) error { return w.ThenDatabaseStaysAtVersion(c, SchemaVersion(v)) })
	ctx.Step(`^migrate reports "([^"]*)"$`, func(t string) error { return w.ThenMigrateReports(OutputToken(t)) })
	ctx.Step(`^migrate refuses naming the missing privilege and "no changes applied"$`,
		func() error { return w.ThenMigrateRefusesNamingTheMissingPrivilege() })
	ctx.Step(`^migrate refuses naming (.+)$`, func(list string) error { return w.ThenMigrateRefuses(quoted(list)) })
	ctx.Step(`^the app role logs in with "([^"]*)"$`, func(c context.Context, pw string) error { return w.ThenAppRoleLogsIn(c, pw) })
	ctx.Step(`^the app role logs in with "([^"]*)" on "([^"]*)"$`,
		func(c context.Context, pw, db string) error { return w.ThenAppRoleLogsInOn(c, pw, DatabaseName(db)) })
	ctx.Step(`^the app role logs in with the password typed in its (composed|decomposed|fullwidth) form$`,
		func(c context.Context, form string) error {
			return w.ThenAppRoleLogsIn(c, ParsePasswordForm(form).Render())
		})
	ctx.Step(`^the app role is refused with "([^"]*)"$`, func(c context.Context, pw string) error { return w.ThenAppRoleRefused(c, pw) })
	ctx.Step(`^the app role cannot update an entry$`, func(c context.Context) error { return w.ThenAppRoleCannotUpdateEntries(c) })
	ctx.Step(`^the app role can neither update nor delete an entry$`,
		func(c context.Context) error { return w.ThenAppRoleCannotChangeOrDeleteEntries(c) })
	ctx.Step(`^no output contains "([^"]*)"$`, func(c context.Context, s string) error { return w.ThenNoOutputContains(c, s) })
	ctx.Step(`^the database server log never contains "([^"]*)"$`,
		func(c context.Context, s string) error { return w.ThenServerLogNeverSees(c, s) })
	ctx.Step(`^the demo password was refused at every observation$`, func() error { return w.ThenDemoPasswordRefusedEveryTime() })
	ctx.Step(`^the interrupted migration (\d+) is flagged unfinished and left no partial change$`,
		func(c context.Context, v int) error { return w.ThenVersionLeftDirty(c, SchemaVersion(v)) })
	ctx.Step(`^the cluster is unchanged$`, func(c context.Context) error { return w.ThenClusterUnchanged(c) })
	ctx.Step(`^no ledger data and no application login exist$`, func(c context.Context) error { return w.ThenNoLedgerTableAndNoAppRole(c) })
	ctx.Step(`^the cluster changed only by the new app role and the new ledger schema$`,
		func(c context.Context) error { return w.ThenOnlyTheFreshAppRoleAndSchemaChanged(c) })
	ctx.Step(`^the history is untouched and the trial balance is zero$`,
		func(c context.Context) error { return w.ThenHistoryUntouchedAfterUpgrade(c) })

	// --- serve ----------------------------------------------------------------------------
	ctx.Step(`^no database is reachable$`, func() error { return w.GivenNoDatabaseReachable() })
	ctx.Step(`^the operator key is "([^"]*)"$`, func(v string) error { return w.GivenServeValue(VarOperatorKey, v) })
	ctx.Step(`^the demo tenant key is "([^"]*)"$`, func(v string) error { return w.GivenServeValue(VarDemoTenantKey, v) })
	ctx.Step(`^(the operator key|the app connection setting|the demo tenant key) is (unset|empty|whitespace-only|set)$`,
		func(v, state string) error { return w.GivenServeSetting(ParseVariable(v), ParseValueState(state)) })
	ctx.Step(`^the migration credential is present in the environment$`, func() error { return w.GivenMigrationCredentialPresent() })
	ctx.Step(`^ledgerops starts$`, func(c context.Context) error { return w.StartLedgerops(c) })
	ctx.Step(`^ledgerops refuses to start naming (.+)$`, func(list string) error { return w.ThenRefusesToStart(quoted(list)) })
	ctx.Step(`^no connection to the database was attempted$`, func() error { return w.ThenNoDatabaseConnectionAttempted() })
	ctx.Step(`^ledgerops reports healthy$`, func(c context.Context) error { return w.ThenReportsHealthy(c) })
	ctx.Step(`^the health answer carries no ledger data$`, func(c context.Context) error { return w.ThenHealthIsBare(c) })
	ctx.Step(`^the operator key is accepted for provisioning a tenant$`, func(c context.Context) error { return w.ThenOperatorKeyAccepted(c) })
	ctx.Step(`^the token "([^"]*)" is refused as unidentified$`, func(c context.Context, t string) error { return w.ThenTokenRefused(c, t) })
	ctx.Step(`^the token "([^"]*)" resolves to the pre-existing demo tenant$`,
		func(c context.Context, t string) error { return w.ThenTokenResolvesToTheLegacyTenant(c, t) })
	ctx.When(`^(?:the operator|Priya) posts a first transfer from "([^"]*)" to "([^"]*)"$`,
		func(c context.Context, from, to string) error { return w.PostFirstTransfer(c, from, to) })
	ctx.Step(`^the transfer is answered with a transaction id$`, func() error { return w.ThenTransferAnsweredWithATransactionID() })
	ctx.Step(`^the operator runs "([^"]*)"$`, func(c context.Context, cmd string) error { return w.RunCommand(c, cmd) })
	ctx.Step(`^the command refuses naming (.+)$`, func(list string) error { return w.ThenCommandRefusesNaming(quoted(list)) })

	// --- version and console ------------------------------------------------------------------
	ctx.Step(`^the binary is built for release "([^"]*)" at commit "([^"]*)"$`,
		func(v, commit string) error { return w.GivenBinaryBuiltFor(v, commit) })
	ctx.Step(`^the binary is built without version information$`, func() error { return w.GivenBinaryBuiltWithoutVersion() })
	ctx.Step(`^it prints "([^"]*)"$`, func(t string) error { return w.ThenItPrints(t) })
	ctx.Step(`^a console bundle that is (absent|empty|without an index page|present)$`,
		func(state string) error { return w.GivenConsoleBundle(ParseConsoleBundle(state)) })
	ctx.Step(`^the binary is built for the image$`, func(c context.Context) error { return w.WhenBuiltForTheImage(c) })
	ctx.Step(`^the build fails$`, func() error { return w.ThenBuildFails() })
	ctx.Step(`^an empty working directory$`, func() error { return w.GivenEmptyWorkingDirectory() })
	ctx.Step(`^an untagged build with a console bundle in the working directory$`,
		func() error { return w.GivenUntaggedBuildWithBundleInWorkingDirectory() })
	ctx.Step(`^the console page is returned$`, func(c context.Context) error { return w.ThenConsolePageIsReturned(c) })
	ctx.Step(`^the console verdict needs the operator key$`, func(c context.Context) error { return w.ThenVerdictNeedsTheOperatorKey(c) })
	ctx.Step(`^the image build copies the console bundle from its own stage and is tagged to embed it$`,
		func() error { return w.ThenConsoleStageFeedsTheGoBuild() })
	ctx.Step(`^the image build context excludes sources and dependencies it does not need$`,
		func() error { return w.ThenDockerIgnoreKeepsTheContextSmall() })

	// --- classifier -----------------------------------------------------------------------------
	ctx.Step(`^no versions have been published yet$`, func() error { return w.GivenNoVersionsPublishedYet() })
	ctx.Step(`^the published tags are (.+)$`, func(list string) error { return w.GivenPublishedTagsAre(quoted(list)) })
	ctx.Step(`^the tag "([^"]*)" is pushed$`, func(c context.Context, t string) error { return w.WhenTagPushed(c, ReleaseTag(t)) })
	ctx.Step(`^a published version "([^"]*)" whose image was built from commit "([^"]*)"$`,
		func(c context.Context, v, commit string) error {
			return w.GivenPublishedVersionBuiltFromCommit(c, v, commit)
		})
	ctx.Step(`^the release plan runs for the resumed tag "([^"]*)" at commit "([^"]*)"$`,
		func(c context.Context, t, commit string) error {
			return w.WhenReleasePlanRunsForResumedTag(c, ReleaseTag(t), commit)
		})
	ctx.Step(`^the release plan refuses naming "([^"]*)" and "([^"]*)"$`,
		func(a, b string) error { return w.ThenPlanRefusesNamingBothCommits(a, b) })
	ctx.Step(`^the plan publishes (.+)$`, func(list string) error { return w.ThenPlanPublishes(quoted(list)) })
	ctx.Step(`^latest moves$`, func() error { return w.ThenLatest(true) })
	ctx.Step(`^latest does not move$`, func() error { return w.ThenLatest(false) })
	ctx.Step(`^the highest stable version is "([^"]*)"$`, func(v string) error { return w.ThenHighestStableIs(v) })
	ctx.Step(`^the run resumes the published version$`, func() error { return w.ThenPlanIsResumed() })
	ctx.Step(`^the tag is rejected as not a version$`, func() error { return w.ThenTagRejected() })

	// --- pipeline structure ---------------------------------------------------------------------
	ctx.Step(`^a release starts for (.+) and not for (.+)$`,
		func(yes, no string) error { return w.ThenReleaseTriggersOnlyOnVersionTags(quoted(yes), quoted(no)) })
	ctx.Step(`^the continuous-integration workflow runs on branches and can be called by the release$`,
		func() error { return w.ThenCIRunsOnBranchesAndCanBeCalled() })
	ctx.Step(`^every job after classify needs the full test suite$`, func() error { return w.ThenEveryJobAfterClassifyNeedsCI() })
	ctx.Step(`^every job condition begins with not-cancelled and names a success clause per dependency$`,
		func() error { return w.ThenEveryJobIfNamesItsDependencies() })
	ctx.Step(`^the "([^"]*)" job needs (.+)$`,
		func(job, list string) error { return w.ThenJobNeeds(ParseWorkflowJob(job), quoted(list)) })
	ctx.Step(`^only build, sign and promote may write packages, only sign may mint an identity token, and the workflow default is read-only$`,
		func() error { return w.ThenJobsPermissions(nil) })
	ctx.Step(`^every action in "([^"]*)" is pinned to a full commit SHA$`, func(p string) error { return w.ThenEveryActionIsPinned(p) })
	ctx.Step(`^no release script interpolates workflow expressions directly$`,
		func() error { return w.ThenRunScriptsNeverInterpolateExpressions() })
	ctx.Step(`^the build pushes the untagged index by digest without provenance or SBOM attestations$`,
		func() error { return w.ThenBuildPushesTheUntaggedIndexByDigest() })
	ctx.Step(`^every run prints the image digest and where it came from$`,
		func() error { return w.ThenRunsReportWhichDigestTheyActedOn() })
	ctx.Step(`^the release is serialised and never cancels a run in progress$`,
		func() error { return w.ThenReleaseIsSerialAndNeverCancelsARun() })
	ctx.Step(`^every base image of the image build is pinned by digest$`, func() error { return w.ThenEveryBaseImageIsPinned() })
	ctx.Step(`^the image build cross-compiles on the build platform and embeds the console$`,
		func() error { return w.ThenImageCrossCompilesAndEmbedsTheConsole() })
	ctx.Step(`^the image runs as a non-root user and has no step after its final base$`,
		func() error { return w.ThenImageRunsNonRootWithNoStepAfterItsFinalBase() })
	ctx.Step(`^a failing signing step stops the release and cannot be bypassed$`, func() error { return w.ThenSigningFailureStopsTheRelease() })
	ctx.Step(`^the published-image example uses no demo credential$`, func() error { return w.ThenExampleComposeUsesNoDemoCredential() })
	ctx.Step(`^the demo compose file runs "ledgerops migrate" with the demo app password set explicitly$`,
		func() error { return w.ThenDemoMigrateRunsTheShippedCommand(composePath) })
	ctx.Step(`^the demo app service never receives the migration credential$`,
		func() error { return w.ThenDemoAppNeverReceivesTheMigrationCredential(composePath) })
	ctx.Step(`^the load-test compose file keeps the app without the migration credential$`,
		func() error { return w.ThenDemoAppNeverReceivesTheMigrationCredential(loadtestComposePath) })

	ctx.Step(`^the release builds linux/amd64 and linux/arm64 and labels the image with its version, commit and source$`,
		func() error { return w.ThenBuildCoversBothArchitecturesWithLabels() })
	ctx.Step(`^the release page template names the image reference, its digest and the verify command$`,
		func() error { return w.ThenReleaseTemplateNamesImageDigestAndVerify() })
	ctx.Step(`^the release job marks prereleases and never lets a backport take the latest badge$`,
		func() error { return w.ThenReleaseJobFlagsPrereleaseAndLatest() })
	ctx.Step(`^the index of "([^"]*)" lists linux/amd64 and linux/arm64 and its labels name the release$`,
		func(c context.Context, tag string) error { return w.ThenIndexListsBothArchitecturesWithLabels(c, tag) })

	// --- promote and smoke against a local registry ------------------------------------------------
	ctx.Step(`^a local registry holding two built images$`, func(c context.Context) error { return w.GivenLocalRegistryWithImages(c) })
	ctx.Step(`^the tags (.+) are already published on (another image|the built image)$`,
		func(c context.Context, list, image string) error { return w.GivenPublishedTags(c, quoted(list), image) })
	ctx.Step(`^promote runs for "([^"]*)" with tags "([^"]*)" moving latest over highest stable "([^"]*)"$`,
		func(c context.Context, v, tags, highest string) error {
			return w.RunPromote(c, v, splitWords(tags), true, highest)
		})
	ctx.Step(`^the tags (.+) resolve to the built image$`,
		func(c context.Context, list string) error { return w.ThenTagsResolveToTheBuiltDigest(c, quoted(list)) })
	ctx.Step(`^promote refuses$`, func() error { return w.ThenPromoteRefuses("") })
	ctx.Step(`^promote refuses saying "([^"]*)"$`, func(t string) error { return w.ThenPromoteRefuses(t) })
	ctx.Step(`^the tag "([^"]*)" still points at another image$`,
		func(c context.Context, t string) error { return w.ThenTagUnchanged(c, t, "another image") })
	ctx.Step(`^no tag "([^"]*)" or "([^"]*)" exists$`,
		func(c context.Context, a, b string) error { return w.ThenNoTagExists(c, []string{a, b}) })
	ctx.Step(`^the smoke check runs against the repository image$`, func(c context.Context) error { return w.RunSmoke(c, true) })
	ctx.Step(`^the smoke check runs against an unrelated image$`, func(c context.Context) error { return w.RunSmoke(c, false) })
	ctx.Step(`^the smoke check reports its pull-to-healthy seconds$`, func() error { return w.ThenSmokeReportsSeconds() })
	ctx.Step(`^the smoke check fails at the migration step$`, func() error { return w.ThenSmokeFailsAtTheMigrationStep() })
	ctx.Step(`^no version tag exists for "([^"]*)" on the registry$`,
		func(c context.Context, v string) error { return w.ThenNoVersionTagOnTheRegistry(c, v) })

	// --- provenance (external where it must be) -------------------------------------------------------
	ctx.Step(`^the real release pipeline has published "([^"]*)"$`, func(string) error { return w.RequireExternal() })
	ctx.Step(`^the published digest of "([^"]*)" is unchanged$`,
		func(string) error { return w.ThenPublishedDigestUnchangedOnTheRealRegistry() })
	ctx.When(`^the tag "([^"]*)" is moved to another commit and pushed$`, func(string) error { return w.WhenTagMovedAndPushedOnTheRealRegistry() })
	ctx.When(`^the quickstart commands are run in order against that release$`, func() error { return w.WhenQuickstartCommandsRunAgainstTheRelease() })
	ctx.Step(`^the quickstart's first transfer is answered with a transaction id$`, func() error { return w.ThenQuickstartTransferAnswered() })
	ctx.Step(`^the published identity matches (.+)$`, func(list string) error { return w.ThenIdentityPatternMatches(quoted(list), true) })
	ctx.Step(`^the published identity does not match (.+)$`, func(list string) error { return w.ThenIdentityPatternMatches(quoted(list), false) })
	ctx.Step(`^cosign verifies "([^"]*)" against the project identity$`,
		func(c context.Context, ref string) error { return w.ThenCosignVerifies(c, ref, true) })
	ctx.Step(`^cosign does not verify "([^"]*)" against the project identity$`,
		func(c context.Context, ref string) error { return w.ThenCosignVerifies(c, ref, false) })
	ctx.Step(`^the SBOM attestation of the amd64 digest of "([^"]*)" verifies$`,
		func(c context.Context, tag string) error { return w.ThenSBOMAttestationVerifies(c, tag) })

	// --- documentation ----------------------------------------------------------------------------------
	ctx.Step(`^the README quickstart leads with the published image$`, func() error { return w.ThenReadmeLeadsWithThePublishedImage() })
	ctx.Step(`^the README no longer lists "([^"]*)" as a limitation$`,
		func(t string) error { return w.ThenDocOmits(readmePath, []string{t}) })
	ctx.Step(`^the quickstart generates the password and keys, steers to the password file and warns against demo credentials$`,
		func() error { return w.ThenQuickstartGeneratesSecretsAndWarns() })
	ctx.Step(`^the quickstart warns that statement logging exposes the password verifier to offline attack$`,
		func() error { return w.ThenQuickstartWarnsAboutStatementLogging() })
	ctx.Step(`^the quickstart tells the operator to migrate before restarting the app$`,
		func() error { return w.ThenQuickstartOrdersMigrateBeforeRestart() })
	ctx.Step(`^the quickstart shows how to verify the signature and the per-platform software bill of materials with the project identity$`,
		func() error { return w.ThenQuickstartShowsVerification() })
	ctx.Step(`^the quickstart explains recovery from an interrupted migration$`,
		func() error { return w.ThenQuickstartExplainsInterruptedMigrationRecovery() })
	ctx.Step(`^the quickstart states the one-database-per-cluster rule and its remedy$`,
		func() error { return w.ThenQuickstartStatesOneDatabasePerCluster() })
	ctx.Step(`^the quickstart recommends pinning by digest and explains the registry cannot enforce tag immutability$`,
		func() error { return w.ThenQuickstartRecommendsDigestPinning() })
}
