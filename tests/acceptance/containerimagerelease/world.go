package containerimagerelease

// World is the acceptance suite's composition root (Tier A, Mandate 10). It
// drives the PRODUCTION entry points: the `ledgerops` binary (or the built
// image for @image scenarios) as a subprocess against a real PostgreSQL 16
// from Testcontainers. Only the port-exposed surface is observed: exit codes,
// output, HTTP answers, logins, and the catalog snapshot (state-delta).
//
// State machine exercised (C2 of the AT completeness taxonomy):
//
//	DATABASE  empty --migrate--> migrated(version 6, app role LOGIN)
//	          empty --killed between files--> claimed(NOLOGIN, version k) --migrate--> migrated
//	          empty --killed in a file--> dirty(k)  --migrate--> refused (dirty), unchanged
//	          legacy(app-secret) --migrate--> migrated(own password)
//	          migrated --migrate again--> migrated (no-op) | rotated
//	CLUSTER   no role --first migrate claims--> role owned by one database
//	          role owned --migrate other empty database--> refused
//	SERVE     misconfigured --start--> refused(exit 1) ; configured --start--> serving
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"ledgerops/tests/common/statedelta"
)

type setting struct {
	state ValueState
	value string
}

type envOverride struct {
	state ValueState
	value string
}

// World holds one scenario's state.
type World struct {
	tags     map[string]bool
	useImage bool
	runner   Runner

	cluster *Cluster
	target  DatabaseName
	tracked []DatabaseName
	owner   DatabaseName

	privUser, privPassword string

	password     string
	inline       setting
	fileSetting  setting
	fileEnding   LineEnding
	fileAccess   SecretFileAccess
	passwordFile string
	files        []MountedFile
	tmpDir       string

	before statedelta.Snapshot
	after  statedelta.Snapshot

	last    RunResult
	outputs []string
	runs    []RunResult
	secrets []string

	// serve
	serveOverrides map[Variable]envOverride
	serveProc      *Process
	serveListening bool
	serveURL       string
	tripwire       *Tripwire
	lastStatus     int
	lastBody       string
	tenantKey      string

	// watching for the demo password during migrate (Rule A)
	observations atomic.Int64
	successes    atomic.Int64

	// concurrency
	concurrent []RunResult

	// version / console
	variant       BinaryVariant
	dsnPassword   string
	gap           PrivilegeGap
	transactionID string
	bundle        ConsoleBundle
	buildErr      error
	bundleDir     string
	consoleCwd    string
	versionText   string

	// release
	rel releaseState
}

// NewWorld builds an unconfigured world; Configure applies the scenario tags.
func NewWorld() *World {
	w := &World{tags: map[string]bool{}, serveOverrides: map[Variable]envOverride{}, fileAccess: WorldReadable, runner: hostRunner{}}
	dir, _ := os.MkdirTemp("", "ledgerops-at-")
	w.tmpDir = dir
	return w
}

// Configure selects the host-binary or image runner from the scenario tags.
func (w *World) Configure(tags []string) {
	for _, t := range tags {
		w.tags[t] = true
	}
	w.useImage = w.tags["@image"]
	if w.useImage {
		w.runner = imageRunner{}
	}
}

// Close releases everything the scenario started.
func (w *World) Close() {
	if w.serveProc != nil {
		w.serveProc.Kill()
	}
	if w.tripwire != nil {
		w.tripwire.Close()
	}
	w.cluster.Stop()
	w.rel.close()
	if w.tmpDir != "" {
		_ = os.RemoveAll(w.tmpDir)
	}
}

// ---- recording ----------------------------------------------------------------

func (w *World) record(r RunResult) {
	w.last = r
	w.runs = append(w.runs, r)
	w.outputs = append(w.outputs, r.Output())
}

func (w *World) trackSecret(s string) {
	if s != "" {
		w.secrets = append(w.secrets, s)
	}
}

func (w *World) corpus(ctx context.Context) string {
	all := strings.Join(w.outputs, "\n")
	if w.serveProc != nil {
		r := w.serveProc.Result()
		all += "\n" + r.Output()
	}
	return all
}

// ---- givens: cluster and credentials ----------------------------------------------

func (w *World) GivenEmptyDatabase(ctx context.Context, db DatabaseName, role RoleName) error {
	cl, err := StartCluster(ctx, string(role), privilegedPassword, string(db), false)
	if err != nil {
		return err
	}
	return w.adoptCluster(ctx, cl, db, string(role), privilegedPassword)
}

func (w *World) adoptCluster(ctx context.Context, cl *Cluster, db DatabaseName, user, password string) error {
	w.cluster, w.target, w.privUser, w.privPassword = cl, db, user, password
	w.tracked = []DatabaseName{db}
	snap, err := cl.Snapshot(ctx, w.tracked...)
	w.before = snap
	return err
}

func (w *World) GivenDdlLoggingCluster(ctx context.Context, db DatabaseName, role RoleName) error {
	cl, err := StartCluster(ctx, string(role), privilegedPassword, string(db), true)
	if err != nil {
		return err
	}
	return w.adoptCluster(ctx, cl, db, string(role), privilegedPassword)
}

func (w *World) GivenPassword(password string) error {
	w.password = password
	w.inline = setting{HasValue, password}
	w.trackSecret(password)
	return nil
}

func (w *World) GivenPasswordForm(form PasswordForm) error {
	return w.GivenPassword(form.Render())
}

func (w *World) GivenPasswordFile(content string, ending LineEnding) error {
	w.password = content
	w.inline = setting{Unset, ""}
	w.fileSetting = setting{HasValue, content}
	w.fileEnding = ending
	w.trackSecret(content)
	return nil
}

func (w *World) GivenPasswordSetting(state ValueState) error {
	w.inline = setting{state, state.Literal()}
	w.password = ""
	return nil
}

func (w *World) GivenBothSources(inline, file string) error {
	w.password = inline
	w.inline = setting{HasValue, inline}
	w.fileSetting = setting{HasValue, file}
	w.fileEnding = LineFeed
	w.trackSecret(inline)
	w.trackSecret(file)
	return nil
}

func (w *World) GivenFileAccess(access SecretFileAccess) error {
	w.fileAccess = access
	return nil
}

// ---- migrate -------------------------------------------------------------------------

func (w *World) migrateInvocation() (Invocation, error) {
	env := map[string]string{
		string(VarMigrateDSN): w.cluster.DSN(string(w.target), w.privUser, w.privPassword, w.useImage),
	}
	w.trackSecret(w.privPassword)
	inv := Invocation{Args: []string{"migrate"}, Env: env, Timeout: 90 * time.Second}
	switch w.inline.state {
	case Unset:
	default:
		env[string(VarAppPassword)] = w.inline.value
	}
	if w.fileSetting.state == HasValue {
		path := filepath.Join(w.tmpDir, "ledgerops_app_password")
		mode := os.FileMode(0o644)
		if w.fileAccess == OwnerOnly {
			mode = 0o400
		}
		_ = os.Remove(path)
		if err := os.WriteFile(path, []byte(w.fileSetting.value+string(w.fileEnding)), mode); err != nil {
			return inv, err
		}
		if w.useImage {
			containerPath := "/run/secrets/ledgerops_app_password"
			env[string(VarAppPasswordFile)] = containerPath
			inv.Files = append(inv.Files, MountedFile{HostPath: path, ContainerPath: containerPath})
		} else {
			env[string(VarAppPasswordFile)] = path
		}
	}
	return inv, nil
}

func (w *World) RunMigrate(ctx context.Context) error {
	inv, err := w.migrateInvocation()
	if err != nil {
		return err
	}
	res, err := w.runner.Run(ctx, inv)
	if err != nil {
		return err
	}
	w.record(res)
	return nil
}

// RunMigrateWatched runs migrate while a poller keeps trying to log in with
// the demo password (Rule A: refused at every observation).
func (w *World) RunMigrateWatched(ctx context.Context) error {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ok, err := w.cluster.LoginAccepted(ctx, string(w.target), appRole, DemoPassword)
			if err == nil {
				w.observations.Add(1)
				if ok {
					w.successes.Add(1)
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	err := w.RunMigrate(ctx)
	// one more observation after migrate returned
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	return err
}

// RunMigrateKilledAt runs the real migrate binary and kills it at the named
// point. The "before" snapshot is the one taken when the cluster was adopted
// unless snapshotAfter is set (the kill is then a Given and its result is the
// state under test).
func (w *World) RunMigrateKilledAt(ctx context.Context, point KillPoint, snapshotAfter bool) error {
	switch point {
	case BetweenFiles:
		// Hold the write that marks file 4 as running (golang-migrate's
		// SetVersion(4, dirty)), inside its own transaction: killing there
		// aborts it, leaving file 3 complete and file 4 not started.
		if err := w.cluster.HoldBeforeVersion(ctx, string(w.target), 4, 120); err != nil {
			return err
		}
	case DuringAFile:
		if err := w.cluster.HoldOn(ctx, string(w.target), "CREATE TABLE", "public.tenants", 120); err != nil {
			return err
		}
	case BeforeTheSet:
		if err := w.cluster.HoldOn(ctx, string(w.target), "ALTER TABLE", "public.transfer_state", 120); err != nil {
			return err
		}
	}
	return w.runKilled(ctx, snapshotAfter)
}

func (w *World) runKilled(ctx context.Context, snapshotAfter bool) error {
	inv, err := w.migrateInvocation()
	if err != nil {
		return err
	}
	proc, err := w.runner.Start(ctx, inv)
	if err != nil {
		return err
	}
	pid, perr := w.cluster.BlockedBackendPID(ctx, string(w.target), proc.Exited)
	if perr != nil {
		proc.Kill()
		w.record(proc.Result())
		return fmt.Errorf("MISSING_FUNCTIONALITY: migrate never reached the hold point (%v); its output: %s", perr, proc.Result().Output())
	}
	if err := w.cluster.TerminateBackend(ctx, pid); err != nil {
		return err
	}
	proc.Kill()
	w.record(proc.Result())
	if err := w.cluster.ReleaseHolds(ctx, string(w.target)); err != nil {
		return err
	}
	if snapshotAfter {
		snap, err := w.cluster.Snapshot(ctx, w.tracked...)
		w.before = snap
		return err
	}
	return nil
}

func (w *World) GivenFailureSituation(ctx context.Context, f FailureSituation) error {
	switch f {
	case NoRoleCreationRight:
		gap := MissingCreateRole
		user, pw, err := w.cluster.ManagedRole(ctx, string(w.target), &gap, false)
		w.privUser, w.privPassword = user, pw
		return err
	case DirtyPreviousRun:
		return w.RunMigrateKilledAt(ctx, DuringAFile, true)
	case DatabaseUnreachable:
		w.target = "no_such_database"
		return nil
	case ConflictingSources:
		return w.GivenBothSources(w.password, "another-"+w.password)
	case SecondDatabase:
		if err := w.RunMigrate(ctx); err != nil {
			return err
		}
		if w.last.ExitCode != 0 {
			return fmt.Errorf("MISSING_FUNCTIONALITY: precondition not met, migrate did not prepare the first database (exit %d): %s", w.last.ExitCode, w.last.Output())
		}
		return w.GivenSecondEmptyDatabaseAfterOwner(ctx, "kestrel_ledger_2")
	}
	return fmt.Errorf("unhandled situation %s", f)
}

// ---- assertions: migrate -------------------------------------------------------------

func has(output, token string) bool {
	return strings.Contains(strings.ToLower(output), strings.ToLower(token))
}

func (w *World) ThenMigrateSucceeds(version SchemaVersion) error {
	if w.last.TimedOut || w.last.ExitCode != 0 {
		return fmt.Errorf("expected migrate to succeed, exit %d (timed out: %v); output: %s", w.last.ExitCode, w.last.TimedOut, w.last.Output())
	}
	want := fmt.Sprintf("%s %d", TokenSchemaAt, version)
	if !has(w.last.Output(), want) {
		return fmt.Errorf("expected the output to report %q, got: %s", want, w.last.Output())
	}
	return nil
}

func (w *World) ThenMigrateReports(token OutputToken) error {
	if w.last.ExitCode != 0 && !has(w.last.Output(), string(token)) {
		return fmt.Errorf("expected exit 0 reporting %q, got exit %d: %s", token, w.last.ExitCode, w.last.Output())
	}
	if w.last.ExitCode != 0 {
		return fmt.Errorf("expected exit 0, got %d: %s", w.last.ExitCode, w.last.Output())
	}
	if !has(w.last.Output(), string(token)) {
		return fmt.Errorf("expected the output to say %q, got: %s", token, w.last.Output())
	}
	return nil
}

func (w *World) ThenMigrateRefuses(tokens []string) error {
	if w.last.TimedOut || w.last.ExitCode == 0 {
		return fmt.Errorf("expected migrate to refuse (non-zero exit), got exit %d (timed out: %v); output: %s", w.last.ExitCode, w.last.TimedOut, w.last.Output())
	}
	for _, t := range tokens {
		if !has(w.last.Output(), RefusalToken(t)) {
			return fmt.Errorf("expected the refusal to name %q, got: %s", t, w.last.Output())
		}
	}
	return nil
}

func (w *World) ThenMigrateRefusesNamingUnfinished(version SchemaVersion) error {
	return w.ThenMigrateRefuses([]string{string(TokenDirty), fmt.Sprint(int(version))})
}

func (w *World) ThenMigrateRefusesSayingPasswordUnchanged() error {
	return w.ThenMigrateRefuses([]string{string(TokenPasswordUnchanged)})
}

func (w *World) ThenOutputSays(token OutputToken) error {
	if !has(w.last.Output(), string(token)) {
		return fmt.Errorf("expected the output to say %q, got: %s", token, w.last.Output())
	}
	return nil
}

func (w *World) ThenAppRoleLogsIn(ctx context.Context, password string) error {
	ok, err := w.cluster.LoginAccepted(ctx, string(w.target), appRole, password)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("expected %s to log in with the supplied password, but the login was refused", appRole)
	}
	return nil
}

func (w *World) ThenAppRoleRefused(ctx context.Context, password string) error {
	ok, err := w.cluster.LoginAccepted(ctx, string(w.target), appRole, password)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("expected %s to be refused with %q, but the login was accepted", appRole, password)
	}
	return nil
}

func (w *World) ThenAppRoleCannotUpdateEntries(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, w.cluster.DSN(string(w.target), appRole, w.password, false))
	if err != nil {
		return fmt.Errorf("the app role could not connect to attempt the update: %w", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE entries SET amount_minor = amount_minor WHERE false`); err == nil {
		return errors.New("the app role was able to UPDATE entries: append-only guarantee is not enforced")
	}
	return nil
}

func (w *World) ThenAppRoleCannotChangeOrDeleteEntries(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, w.cluster.DSN(string(w.target), appRole, w.password, false))
	if err != nil {
		return fmt.Errorf("the app role could not connect: %w", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE entries SET amount_minor = amount_minor WHERE false`); err == nil {
		return errors.New("UPDATE on entries was permitted")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM entries WHERE false`); err == nil {
		return errors.New("DELETE on entries was permitted")
	}
	return nil
}

func (w *World) ThenNoOutputContains(ctx context.Context, secret string) error {
	if strings.Contains(w.corpus(ctx), secret) {
		return fmt.Errorf("the secret %q appears in program output", secret)
	}
	return nil
}

func (w *World) ThenNoSecretInAnyOutput(ctx context.Context) error {
	for _, s := range w.secrets {
		if err := w.ThenNoOutputContains(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) ThenServerLogNeverSees(ctx context.Context, secret string) error {
	if strings.Contains(w.cluster.ServerLog(ctx), secret) {
		return fmt.Errorf("the plaintext %q appears in the database server log", secret)
	}
	return nil
}

func (w *World) ThenDemoPasswordRefusedEveryTime() error {
	n, ok := w.observations.Load(), w.successes.Load()
	if n < 3 {
		return fmt.Errorf("only %d login observations were made while migrate ran: too few to prove anything", n)
	}
	if ok != 0 {
		return fmt.Errorf("the demo password was accepted in %d of %d observations", ok, n)
	}
	return nil
}

// ---- state-delta ---------------------------------------------------------------------

func (w *World) assertDelta(ctx context.Context, universe []string, expected map[string]statedelta.Predicate) error {
	after, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	w.after = after
	tb := &recordingTB{}
	statedelta.AssertStateDelta(tb, w.before, after, universe, expected)
	if tb.message != "" {
		return errors.New(tb.message)
	}
	return nil
}

func (w *World) ThenClusterUnchanged(ctx context.Context) error {
	return w.assertDelta(ctx, SnapshotUniverse(w.tracked...), nil)
}

func (w *World) ThenOnlyTheFreshAppRoleAndSchemaChanged(ctx context.Context) error {
	p := "db." + string(w.target) + "."
	return w.assertDelta(ctx, SnapshotUniverse(w.tracked...), map[string]statedelta.Predicate{
		"role.ledgerops_app.exists":               statedelta.SetTo(true),
		"role.ledgerops_app.can_login":            statedelta.SetTo(true),
		"role.ledgerops_app.password_fingerprint": Changed(),
		"role.ledgerops_app.owner_marker":         ContainsText(string(w.target)),
		p + "acl":                                 Changed(),
		p + "schema_version":                      statedelta.SetTo(6),
		p + "tables":                              Changed(),
	})
}

func (w *World) ThenHistoryUntouchedAfterUpgrade(ctx context.Context) error {
	p := "db." + string(w.target) + "."
	universe := []string{
		"roles.others.fingerprint", "databases",
		"role.ledgerops_app.can_login", "role.ledgerops_app.password_fingerprint", "role.ledgerops_app.owner_marker",
		p + "schema_version", p + "dirty", p + "entries.count", p + "trial_balance.imbalance_minor", p + "balances",
	}
	return w.assertDelta(ctx, universe, map[string]statedelta.Predicate{
		"role.ledgerops_app.password_fingerprint": Changed(),
		"role.ledgerops_app.owner_marker":         ContainsText(string(w.target)),
		p + "schema_version":                      statedelta.SetTo(6),
	})
}

func (w *World) ThenNothingChangedExceptTheCurrentVersion(ctx context.Context) error {
	return w.ThenClusterUnchanged(ctx)
}

func (w *World) ThenNoLedgerTableAndNoAppRole(ctx context.Context) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	p := "db." + string(w.target) + "."
	if snap["role.ledgerops_app.exists"] == true {
		return errors.New("the app role exists after a refused run")
	}
	if snap[p+"tables"] != "" {
		return fmt.Errorf("tables exist after a refused run: %v", snap[p+"tables"])
	}
	return nil
}

func (w *World) thenDirtyVersionNamed(version SchemaVersion) error {
	if !has(w.last.Output(), TokenDirty.String()) || !strings.Contains(w.last.Output(), fmt.Sprint(int(version))) {
		return fmt.Errorf("expected the refusal to name dirty version %d, got: %s", version, w.last.Output())
	}
	return nil
}

// String lets tokens be passed where text is needed.
func (t OutputToken) String() string { return string(t) }

func (w *World) ThenVersionLeftDirty(ctx context.Context, version SchemaVersion) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	p := "db." + string(w.target) + "."
	if tables := fmt.Sprint(snap[p+"tables"]); version == 3 && strings.Contains(tables, "tenants") {
		return fmt.Errorf("the killed file left a partial effect (tenants exists): %s", tables)
	}
	if snap[p+"dirty"] != true || snap[p+"schema_version"] != int(version) {
		return fmt.Errorf("expected version %d left dirty (premise of DDD-29), got version %v dirty %v", version, snap[p+"schema_version"], snap[p+"dirty"])
	}
	return nil
}

// ---- legacy database --------------------------------------------------------------------

func (w *World) GivenLegacyDatabase(ctx context.Context, version SchemaVersion, entries int) error {
	cl, err := StartCluster(ctx, legacyPrivileged, legacyPrivPassword, legacyDatabase, false)
	if err != nil {
		return err
	}
	w.cluster = cl
	if err := cl.ApplyFrozen(ctx, uint(version)); err != nil {
		return err
	}
	if err := cl.SeedHistory(ctx, legacyDatabase, entries/2); err != nil {
		return err
	}
	return w.adoptCluster(ctx, cl, legacyDatabase, legacyPrivileged, legacyPrivPassword)
}

func (w *World) ThenLegacyAcceptsDemoPassword(ctx context.Context) error {
	return w.ThenAppRoleLogsIn(ctx, DemoPassword)
}

func (w *World) ThenSeededHistoryPresent(ctx context.Context, entries int) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	p := "db." + string(w.target) + "."
	if snap[p+"entries.count"] != entries || snap[p+"trial_balance.imbalance_minor"] != 0 {
		return fmt.Errorf("expected %d entries and a zero trial balance, got %v and %v", entries, snap[p+"entries.count"], snap[p+"trial_balance.imbalance_minor"])
	}
	return nil
}

func (w *World) GivenInjectedFailureInLaterMigration(ctx context.Context) error {
	return w.cluster.FailOn(ctx, string(w.target), "public.counterparty_aliases")
}

// ---- privilege fixtures ------------------------------------------------------------------

func (w *World) GivenPrivilegedRoleWithGap(ctx context.Context, gap PrivilegeGap) error {
	w.gap = gap
	if gap.NeedsExistingRole() {
		if err := w.GivenLegacyDatabase(ctx, 4, 40); err != nil {
			return err
		}
	} else if err := w.GivenEmptyDatabase(ctx, "kestrel_ledger", "kestrel_admin"); err != nil {
		return err
	}
	user, pw, err := w.cluster.ManagedRole(ctx, string(w.target), &gap, gap.NeedsExistingRole())
	w.privUser, w.privPassword = user, pw
	if err != nil {
		return err
	}
	return w.resnapshot(ctx)
}

func (w *World) GivenPrivilegedRoleWithMinimum(ctx context.Context) error {
	if err := w.GivenEmptyDatabase(ctx, "kestrel_ledger", "kestrel_admin"); err != nil {
		return err
	}
	user, pw, err := w.cluster.ManagedRole(ctx, string(w.target), nil, false)
	w.privUser, w.privPassword = user, pw
	if err != nil {
		return err
	}
	return w.resnapshot(ctx)
}

func (w *World) GivenPrivilegedRoleThatMayGrantConnectButDoesNotOwn(ctx context.Context) error {
	if err := w.GivenEmptyDatabase(ctx, "kestrel_ledger", "kestrel_admin"); err != nil {
		return err
	}
	gap := NotDatabaseOwner
	user, pw, err := w.cluster.ManagedRole(ctx, string(w.target), &gap, false)
	if err != nil {
		return err
	}
	w.privUser, w.privPassword = user, pw
	if err := w.cluster.Exec(ctx, "postgres", fmt.Sprintf(`GRANT CONNECT ON DATABASE "%s" TO %s WITH GRANT OPTION`, w.target, user)); err != nil {
		return err
	}
	if err := w.cluster.Exec(ctx, string(w.target), fmt.Sprintf(`GRANT CREATE ON SCHEMA public TO %s`, user)); err != nil {
		return err
	}
	return w.resnapshot(ctx)
}

func (w *World) ThenMigrateRefusesNamingTheMissingPrivilege() error {
	return w.ThenMigrateRefuses(append(w.gap.NamedBy(), string(TokenNoChangesApplied)))
}

func (w *World) ThenAppRoleLogsInOn(ctx context.Context, password string, db DatabaseName) error {
	w.target = db
	return w.ThenAppRoleLogsIn(ctx, password)
}

func (w *World) ThenOldPasswordStillWorks(ctx context.Context) error {
	return w.ThenAppRoleLogsIn(ctx, DemoPassword)
}

// ---- cluster rule -------------------------------------------------------------------------

func (w *World) GivenMigratedDatabase(ctx context.Context, db DatabaseName, password string) error {
	if err := w.GivenEmptyDatabase(ctx, db, "kestrel_admin"); err != nil {
		return err
	}
	_ = w.GivenPassword(password)
	if err := w.RunMigrate(ctx); err != nil {
		return err
	}
	if w.last.ExitCode != 0 {
		return fmt.Errorf("MISSING_FUNCTIONALITY: precondition not met, migrate did not prepare %q (exit %d): %s", db, w.last.ExitCode, w.last.Output())
	}
	return nil
}

func (w *World) GivenSecondEmptyDatabaseAfterOwner(ctx context.Context, second DatabaseName) error {
	if w.cluster == nil {
		if err := w.GivenMigratedDatabase(ctx, "kestrel_ledger", ChosenPassword); err != nil {
			return err
		}
	}
	if err := w.cluster.Exec(ctx, "postgres", fmt.Sprintf(`CREATE DATABASE "%s"`, second)); err != nil {
		return err
	}
	w.owner = w.target
	w.tracked = []DatabaseName{w.owner, second}
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	w.before = snap
	w.target = second
	return err
}

func (w *World) GivenOwnerDatabaseDropped(ctx context.Context, db DatabaseName) error {
	if db != w.owner {
		return fmt.Errorf("scenario error: %s is not the owning database (%s)", db, w.owner)
	}
	if err := w.cluster.Exec(ctx, "postgres", fmt.Sprintf(`DROP DATABASE "%s" WITH (FORCE)`, w.owner)); err != nil {
		return err
	}
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	w.before = snap
	return err
}

func (w *World) GivenAbandonedClaim(ctx context.Context, first, second DatabaseName) error {
	if err := w.GivenEmptyDatabase(ctx, first, "kestrel_admin"); err != nil {
		return err
	}
	if err := w.cluster.Exec(ctx, "postgres",
		`CREATE ROLE ledgerops_app NOLOGIN`,
		fmt.Sprintf(`COMMENT ON ROLE ledgerops_app IS 'ledgerops-owner-database=%s'`, first),
		fmt.Sprintf(`CREATE DATABASE "%s"`, second)); err != nil {
		return err
	}
	w.owner = first
	w.tracked = []DatabaseName{first, second}
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	w.before = snap
	return err
}

func (w *World) GivenTwoEmptyDatabases(ctx context.Context, a, b DatabaseName) error {
	if err := w.GivenEmptyDatabase(ctx, a, "kestrel_admin"); err != nil {
		return err
	}
	if err := w.cluster.Exec(ctx, "postgres", fmt.Sprintf(`CREATE DATABASE "%s"`, b)); err != nil {
		return err
	}
	w.owner = a
	w.tracked = []DatabaseName{a, b}
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	w.before = snap
	return err
}

func (w *World) RunMigrateAgainst(ctx context.Context, db DatabaseName) error {
	w.target = db
	return w.RunMigrate(ctx)
}

func (w *World) RunTwoMigratesTogether(ctx context.Context, a, b DatabaseName) error {
	var procs []*Process
	for _, db := range []DatabaseName{a, b} {
		w.target = db
		inv, err := w.migrateInvocation()
		if err != nil {
			return err
		}
		p, err := w.runner.Start(ctx, inv)
		if err != nil {
			return err
		}
		procs = append(procs, p)
	}
	w.concurrent = nil
	for _, p := range procs {
		res := p.Wait(90 * time.Second)
		w.concurrent = append(w.concurrent, res)
		w.outputs = append(w.outputs, res.Output())
	}
	return nil
}

func (w *World) ThenExactlyOneSucceedsAndTheOtherRefuses() error {
	wins, losses := 0, 0
	for _, r := range w.concurrent {
		switch {
		case r.ExitCode == 0:
			wins++
		case has(r.Output(), TokenRoleExists.String()):
			losses++
		}
	}
	if wins != 1 || losses != 1 {
		return fmt.Errorf("expected exactly one success and one cluster-rule refusal, got %d and %d: %s", wins, losses, w.dumpConcurrent())
	}
	return nil
}

func (w *World) ThenAtLeastOneSucceedsAndBothEndConsistent(ctx context.Context) error {
	wins := 0
	for _, r := range w.concurrent {
		if r.ExitCode == 0 {
			wins++
		}
	}
	if wins < 1 {
		return fmt.Errorf("expected at least one run to succeed: %s", w.dumpConcurrent())
	}
	return nil
}

func (w *World) dumpConcurrent() string {
	var parts []string
	for i, r := range w.concurrent {
		parts = append(parts, fmt.Sprintf("run %d exit %d: %s", i+1, r.ExitCode, tail(r.Output(), 400)))
	}
	return strings.Join(parts, " | ")
}

func (w *World) ThenNeitherDatabaseIsDirty(ctx context.Context) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	for _, db := range w.tracked {
		if snap["db."+string(db)+".dirty"] == true {
			return fmt.Errorf("database %s was left dirty", db)
		}
	}
	return nil
}

func (w *World) ThenOnlyOneDatabaseOwnsTheRole(ctx context.Context) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	marker := fmt.Sprint(snap["role.ledgerops_app.owner_marker"])
	owners := 0
	for _, db := range w.tracked {
		if snap["db."+string(db)+".schema_version"] == 6 {
			owners++
		}
	}
	if owners != 1 || marker == "" {
		return fmt.Errorf("expected exactly one migrated database owning the role, got %d (marker %q)", owners, marker)
	}
	return nil
}

func (w *World) ThenSecondDatabaseUntouched(ctx context.Context) error {
	return w.ThenClusterUnchanged(ctx)
}

// ---- helpers shared with serve ---------------------------------------------------------

func jsonBody(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (w *World) ThenDatabaseStaysAtVersion(ctx context.Context, version SchemaVersion) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	if err != nil {
		return err
	}
	p := "db." + string(w.target) + "."
	if snap[p+"schema_version"] != int(version) || snap[p+"dirty"] == true {
		return fmt.Errorf("expected the database at complete version %d, got version %v (unfinished: %v)", version, snap[p+"schema_version"], snap[p+"dirty"])
	}
	return nil
}

// ---- Given steps that run the real binary as a precondition (walking skeleton) --------

func (w *World) GivenDatabasePrepared(ctx context.Context) error {
	if err := w.RunMigrate(ctx); err != nil {
		return err
	}
	if err := w.ThenMigrateSucceeds(6); err != nil {
		return fmt.Errorf("MISSING_FUNCTIONALITY: precondition not met, the database was not prepared: %w", err)
	}
	return nil
}

func (w *World) GivenLedgerOpsRunning(ctx context.Context) error {
	if err := w.StartLedgerops(ctx); err != nil {
		return err
	}
	if err := w.mustBeServing(); err != nil {
		return fmt.Errorf("MISSING_FUNCTIONALITY: precondition not met: %w", err)
	}
	return nil
}

func (w *World) RunTwoMigratesForTheTrackedDatabases(ctx context.Context) error {
	if len(w.tracked) != 2 {
		return fmt.Errorf("scenario error: expected two tracked databases, have %d", len(w.tracked))
	}
	return w.RunTwoMigratesTogether(ctx, w.tracked[0], w.tracked[1])
}

// resnapshot re-takes the "before" snapshot after a fixture changed the
// cluster (managed role, database owner, ACLs), so "unchanged" compares the
// run under test against the state it started from.
func (w *World) resnapshot(ctx context.Context) error {
	snap, err := w.cluster.Snapshot(ctx, w.tracked...)
	w.before = snap
	return err
}
