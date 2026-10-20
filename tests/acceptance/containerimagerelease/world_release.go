package containerimagerelease

// Release-pipeline side of the World: the pure classifier, parsed-YAML
// structural views of the workflows, Dockerfile and compose files, the
// promote/smoke logic against a local registry:2, documentation checks and
// the @requires_external provenance checks.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

type releaseState struct {
	existing []string
	tag      string
	run      *planRun

	doc      map[string]any
	docText  string
	lastText string

	registry   testcontainers.Container
	registryAt string
	digestA    string
	digestB    string
	promote    RunResult
}

func (r *releaseState) close() {
	if r.registry != nil {
		_ = r.registry.Terminate(context.Background())
	}
}

// ---- classifier (driven through scripts/releaseplan, the real driving port) ----------

type planRun struct {
	exit   int
	output string
	fields map[string]string
}

var (
	planBinOnce sync.Once
	planBinPath string
	planBinErr  error
)

func releasePlanBinary(ctx context.Context) (string, error) {
	planBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ledgerops-releaseplan-")
		if err != nil {
			planBinErr = err
			return
		}
		registerTempDir(dir)
		planBinPath = filepath.Join(dir, "releaseplan")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", planBinPath, "./scripts/releaseplan")
		cmd.Dir = repoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			planBinErr = &BuildFailure{Output: string(out)}
		}
	})
	return planBinPath, planBinErr
}

// runReleasePlan runs `releaseplan --tag T --existing-tags a,b,c [extra...]`
// and parses its `key=value` output lines (contract: upstream-issues U-8).
func (w *World) runReleasePlan(ctx context.Context, tag string, extra ...string) error {
	bin, err := releasePlanBinary(ctx)
	if err != nil {
		return err
	}
	args := append([]string{"--tag", tag, "--existing-tags", strings.Join(w.rel.existing, ",")}, extra...)
	out, runErr := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	run := &planRun{output: string(out), fields: map[string]string{}}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		run.exit = ee.ExitCode()
	}
	for _, line := range strings.Split(string(out), "\n") {
		if kv := strings.SplitN(strings.TrimSpace(line), "=", 2); len(kv) == 2 {
			run.fields[kv[0]] = kv[1]
		}
	}
	w.rel.run = run
	w.outputs = append(w.outputs, run.output)
	return nil
}

func (w *World) GivenNoVersionsPublishedYet() error { w.rel.existing = nil; return nil }

func (w *World) GivenPublishedTagsAre(tags []string) error { w.rel.existing = tags; return nil }

func (w *World) WhenTagPushed(ctx context.Context, tag ReleaseTag) error {
	w.rel.tag = string(tag)
	return w.runReleasePlan(ctx, string(tag))
}

func (w *World) planFields() (map[string]string, error) {
	r := w.rel.run
	if r == nil || r.exit != 0 {
		return nil, fmt.Errorf("MISSING_FUNCTIONALITY: releaseplan did not produce a plan (exit %d): %s", r.exit, strings.TrimSpace(r.output))
	}
	return r.fields, nil
}

func (w *World) ThenPlanPublishes(tags []string) error {
	f, err := w.planFields()
	if err != nil {
		return err
	}
	if got := strings.Fields(f["tags"]); !reflect.DeepEqual(got, tags) {
		return fmt.Errorf("expected tags %v, got %v", tags, got)
	}
	return nil
}

func (w *World) ThenLatest(moves bool) error {
	f, err := w.planFields()
	if err != nil {
		return err
	}
	if f["move_latest"] != fmt.Sprint(moves) {
		return fmt.Errorf("expected move_latest=%v, plan says %q", moves, f["move_latest"])
	}
	return nil
}

func (w *World) ThenHighestStableIs(v string) error {
	f, err := w.planFields()
	if err != nil {
		return err
	}
	if f["highest_stable"] != v {
		return fmt.Errorf("expected highest stable %q, got %q", v, f["highest_stable"])
	}
	return nil
}

func (w *World) ThenPlanIsResumed() error {
	f, err := w.planFields()
	if err != nil {
		return err
	}
	if f["resumed"] != "true" {
		return errors.New("expected the run to resume the already published version")
	}
	return nil
}

func (w *World) ThenTagRejected() error {
	r := w.rel.run
	if r == nil || r.exit == 0 || !has(r.output, TokenNotAVersionTag.String()) {
		return fmt.Errorf("expected %q to be rejected saying %q, got exit %d: %s", w.rel.tag, TokenNotAVersionTag, r.exit, strings.TrimSpace(r.output))
	}
	return nil
}

// ---- resume check: image revision label versus the commit being released ------------

func (w *World) GivenPublishedVersionBuiltFromCommit(ctx context.Context, version, commit string) error {
	dir := filepath.Join(w.tmpDir, "labelled")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dockerfile := "FROM postgres:16\nLABEL org.opencontainers.image.revision=" + commit + "\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	target := w.repo() + ":" + version
	if out, err := exec.CommandContext(ctx, "docker", "build", "-q", "-t", target, dir).CombinedOutput(); err != nil {
		return fmt.Errorf("building the labelled image: %s", out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "push", target).CombinedOutput(); err != nil {
		return fmt.Errorf("pushing the labelled image: %s", out)
	}
	w.rel.existing = []string{version}
	return nil
}

func (w *World) WhenReleasePlanRunsForResumedTag(ctx context.Context, tag ReleaseTag, commit string) error {
	w.rel.tag = string(tag)
	return w.runReleasePlan(ctx, string(tag), "--image-repo", w.repo(), "--commit", commit)
}

func (w *World) ThenPlanRefusesNamingBothCommits(published, releasing string) error {
	r := w.rel.run
	if r == nil || r.exit == 0 {
		return fmt.Errorf("expected the plan to refuse, got exit %d: %s", r.exit, strings.TrimSpace(r.output))
	}
	for _, c := range []string{published, releasing} {
		if !strings.Contains(r.output, c) {
			return fmt.Errorf("expected the refusal to name commit %s, got: %s", c, strings.TrimSpace(r.output))
		}
	}
	return nil
}

// ---- parsed YAML views ---------------------------------------------------------------------

func (w *World) loadYAML(rel string) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), rel))
	if err != nil {
		return nil, fmt.Errorf("MISSING_FUNCTIONALITY: %s does not exist yet: %w", rel, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s is not valid YAML: %w", rel, err)
	}
	w.rel.docText = string(b)
	return doc, nil
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, i := range x {
			out = append(out, fmt.Sprint(i))
		}
		return out
	}
	return nil
}

func (w *World) releaseJobs() (map[string]map[string]any, error) {
	doc, err := w.loadYAML(releaseWorkflowPath)
	if err != nil {
		return nil, err
	}
	w.rel.doc = doc
	jobs := map[string]map[string]any{}
	for name, j := range asMap(doc["jobs"]) {
		jobs[name] = asMap(j)
	}
	return jobs, nil
}

func ghGlobToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			b.WriteString("[^/]*")
		case '+':
			b.WriteString("+")
		case '[':
			j := strings.IndexByte(pattern[i:], ']')
			b.WriteString(pattern[i : i+j+1])
			i += j
		case '.', '(', ')', '?', '^', '$', '|', '{', '}', '\\':
			b.WriteString("\\" + string(c))
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (w *World) ThenReleaseTriggersOnlyOnVersionTags(accepted, rejected []string) error {
	doc, err := w.loadYAML(releaseWorkflowPath)
	if err != nil {
		return err
	}
	on := asMap(doc["on"])
	push := asMap(on["push"])
	patterns := asStrings(push["tags"])
	if len(patterns) == 0 || on["pull_request"] != nil || on["workflow_dispatch"] != nil || push["branches"] != nil {
		return fmt.Errorf("release.yml must trigger on tag push only; got on=%v", on)
	}
	match := func(tag string) bool {
		for _, p := range patterns {
			if ghGlobToRegexp(p).MatchString(tag) {
				return true
			}
		}
		return false
	}
	for _, t := range accepted {
		if !match(t) {
			return fmt.Errorf("tag %q should trigger the release but matches none of %v", t, patterns)
		}
	}
	for _, t := range rejected {
		if match(t) {
			return fmt.Errorf("tag %q must not trigger the release but matches %v", t, patterns)
		}
	}
	return nil
}

func (w *World) ThenCIRunsOnBranchesAndCanBeCalled() error {
	doc, err := w.loadYAML(ciWorkflowPath)
	if err != nil {
		return err
	}
	on := asMap(doc["on"])
	if _, ok := on["workflow_call"]; !ok {
		return errors.New("ci.yml has no workflow_call trigger")
	}
	push := asMap(on["push"])
	if push["tags"] != nil || len(asStrings(push["branches"])) == 0 {
		return fmt.Errorf("ci.yml push must be branches-only, got %v", push)
	}
	jobs := asMap(doc["jobs"])
	for _, j := range []string{"workflow-guards", "container-acceptance"} {
		if jobs[j] == nil {
			return fmt.Errorf("ci.yml has no %s job", j)
		}
	}
	if !contains(asStrings(asMap(jobs["container-acceptance"])["needs"]), "integration") {
		return errors.New("container-acceptance must need integration")
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (w *World) ThenEveryJobAfterClassifyNeedsCI() error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	for name, j := range jobs {
		if name == "classify" || name == "ci" {
			continue
		}
		if !contains(asStrings(j["needs"]), "ci") {
			return fmt.Errorf("job %s does not need ci: a resumed release could skip the suite", name)
		}
	}
	return nil
}

func (w *World) ThenEveryJobIfNamesItsDependencies() error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	for name, j := range jobs {
		needs := asStrings(j["needs"])
		if len(needs) == 0 {
			continue
		}
		cond := strings.Join(strings.Fields(fmt.Sprint(j["if"])), " ")
		if !strings.HasPrefix(cond, "!cancelled()") {
			return fmt.Errorf("job %s: if must start with !cancelled(), got %q", name, cond)
		}
		for _, dep := range needs {
			ok := strings.Contains(cond, "needs."+dep+".result == 'success'")
			if dep == "build" {
				ok = ok || strings.Contains(cond, "needs.build.result == 'skipped'")
			}
			if !ok {
				return fmt.Errorf("job %s: if names no success clause for dependency %s (got %q)", name, dep, cond)
			}
		}
	}
	return nil
}

func (w *World) ThenJobNeeds(job WorkflowJob, deps []string) error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	j, ok := jobs[string(job)]
	if !ok {
		return fmt.Errorf("release.yml has no %s job", job)
	}
	for _, d := range deps {
		if !contains(asStrings(j["needs"]), d) {
			return fmt.Errorf("job %s does not need %s (needs %v)", job, d, asStrings(j["needs"]))
		}
	}
	return nil
}

func (w *World) ThenJobsPermissions(table map[string]string) error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	for name, j := range jobs {
		perms := asMap(j["permissions"])
		granted := map[string]string{}
		for k, v := range perms {
			granted[k] = fmt.Sprint(v)
		}
		for k, v := range granted {
			if v != "write" {
				continue
			}
			if k == "id-token" && name != "sign" {
				return fmt.Errorf("job %s holds id-token: write; only sign may", name)
			}
			if k == "packages" && !contains([]string{"build", "sign", "promote"}, name) {
				return fmt.Errorf("job %s holds packages: write; only build, sign and promote may", name)
			}
		}
		if want, ok := table[name]; ok && granted[strings.Split(want, ":")[0]] != strings.TrimSpace(strings.Split(want, ":")[1]) {
			return fmt.Errorf("job %s: expected %s, got %v", name, want, granted)
		}
	}
	top := asMap(w.rel.doc["permissions"])
	if fmt.Sprint(top["contents"]) != "read" {
		return fmt.Errorf("workflow-level permissions must be contents: read, got %v", top)
	}
	return nil
}

var sha40 = regexp.MustCompile(`@[0-9a-f]{40}(\s|$)`)

func (w *World) ThenEveryActionIsPinned(path string) error {
	if _, err := w.loadYAML(path); err != nil {
		return err
	}
	for i, line := range strings.Split(w.rel.docText, "\n") {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if !strings.HasPrefix(t, "uses:") || strings.Contains(t, "uses: ./") {
			continue
		}
		if !sha40.MatchString(strings.TrimPrefix(t, "uses:") + " ") {
			return fmt.Errorf("%s:%d: %q is not pinned to a full commit SHA", path, i+1, t)
		}
	}
	return nil
}

func (w *World) ThenRunScriptsNeverInterpolateExpressions() error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	for name, j := range jobs {
		steps, _ := j["steps"].([]any)
		for _, s := range steps {
			if run, ok := asMap(s)["run"].(string); ok && strings.Contains(run, "${{") {
				return fmt.Errorf("job %s: a run script interpolates ${{ }} directly (script injection); read it through env", name)
			}
		}
	}
	return nil
}

// stepsUsing returns the `with:` maps of every step of a job that uses the action.
func stepsUsing(job map[string]any, action string) []map[string]any {
	var out []map[string]any
	steps, _ := job["steps"].([]any)
	for _, s := range steps {
		m := asMap(s)
		if strings.HasPrefix(fmt.Sprint(m["uses"]), action+"@") {
			out = append(out, asMap(m["with"]))
		}
	}
	return out
}

func (w *World) buildStep() (map[string]any, error) {
	jobs, err := w.releaseJobs()
	if err != nil {
		return nil, err
	}
	b := stepsUsing(jobs["build"], "docker/build-push-action")
	if len(b) != 1 {
		return nil, fmt.Errorf("the build job must have exactly one docker/build-push-action step, has %d", len(b))
	}
	return b[0], nil
}

func (w *World) ThenBuildPushesTheUntaggedIndexByDigest() error {
	with, err := w.buildStep()
	if err != nil {
		return err
	}
	if !strings.Contains(fmt.Sprint(with["outputs"]), "push-by-digest=true") || fmt.Sprint(with["provenance"]) != "false" || fmt.Sprint(with["sbom"]) != "false" {
		return fmt.Errorf("build step must push by digest with provenance and sbom off, has outputs=%v provenance=%v sbom=%v", with["outputs"], with["provenance"], with["sbom"])
	}
	return nil
}

func (w *World) ThenReleaseIsSerialAndNeverCancelsARun() error {
	if _, err := w.releaseJobs(); err != nil {
		return err
	}
	c := asMap(w.rel.doc["concurrency"])
	if fmt.Sprint(c["group"]) != "release" || fmt.Sprint(c["cancel-in-progress"]) != "false" {
		return fmt.Errorf("expected concurrency {group: release, cancel-in-progress: false}, got %v", c)
	}
	return nil
}

func (w *World) ThenRunsReportWhichDigestTheyActedOn() error {
	if _, err := w.releaseJobs(); err != nil {
		return err
	}
	for _, want := range []string{"image_digest=", "source="} {
		if !strings.Contains(w.rel.docText, want) {
			return fmt.Errorf("release.yml never prints %q", want)
		}
	}
	return nil
}

// ---- Dockerfile and compose --------------------------------------------------------------

func (w *World) readFile(rel string) (string, error) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), rel))
	if err != nil {
		return "", fmt.Errorf("MISSING_FUNCTIONALITY: %s does not exist yet: %w", rel, err)
	}
	w.rel.docText = string(b)
	return w.rel.docText, nil
}

func (w *World) dockerfileLines() ([]string, error) {
	text, err := w.readFile(dockerfilePath)
	if err != nil {
		return nil, err
	}
	return strings.Split(text, "\n"), nil
}

func (w *World) ThenEveryBaseImageIsPinned() error {
	lines, err := w.dockerfileLines()
	if err != nil {
		return err
	}
	for i, l := range lines {
		if f := strings.Fields(l); len(f) > 1 && f[0] == "FROM" {
			ref := f[len(f)-1]
			if len(f) > 3 && f[len(f)-2] == "AS" {
				ref = f[len(f)-3]
			}
			if !strings.Contains(ref, "@sha256:") {
				return fmt.Errorf("Dockerfile:%d: %q carries no digest", i+1, strings.TrimSpace(l))
			}
		}
	}
	return nil
}

func (w *World) ThenImageCrossCompilesAndEmbedsTheConsole() error {
	lines, err := w.dockerfileLines()
	if err != nil {
		return err
	}
	text := strings.Join(lines, "\n")
	for i, l := range lines {
		if f := strings.Fields(l); len(f) > 1 && f[0] == "FROM" && (strings.Contains(l, "golang") || strings.Contains(l, "node")) && !strings.Contains(l, "--platform=$BUILDPLATFORM") {
			return fmt.Errorf("Dockerfile:%d: builder stage must be FROM --platform=$BUILDPLATFORM", i+1)
		}
	}
	for _, want := range []string{"GOOS=$TARGETOS", "GOARCH=$TARGETARCH", "-tags console_embed"} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("Dockerfile lacks %q", want)
		}
	}
	return nil
}

func (w *World) ThenImageRunsNonRootWithNoStepAfterItsFinalBase() error {
	lines, err := w.dockerfileLines()
	if err != nil {
		return err
	}
	last := -1
	for i, l := range lines {
		if f := strings.Fields(l); len(f) > 0 && f[0] == "FROM" {
			last = i
		}
	}
	for _, l := range lines[last+1:] {
		if f := strings.Fields(l); len(f) > 0 && f[0] == "RUN" {
			return fmt.Errorf("a RUN follows the final FROM: %q", strings.TrimSpace(l))
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "USER 65532:65532") {
		return errors.New("Dockerfile does not set USER 65532:65532")
	}
	return nil
}

func (w *World) ThenConsoleStageFeedsTheGoBuild() error {
	text, err := w.readFile(dockerfilePath)
	if err != nil {
		return err
	}
	if !strings.Contains(text, "COPY --from=") || !strings.Contains(text, "console_dist") || !strings.Contains(text, "npm") {
		return errors.New("the Go build stage must copy the bundle built by the Node stage into console_dist, so a failed console build stops the image")
	}
	return nil
}

func (w *World) ThenDockerIgnoreKeepsTheContextSmall() error {
	text, err := w.readFile(".dockerignore")
	if err != nil {
		return err
	}
	for _, want := range []string{".git", "node_modules"} {
		if !strings.Contains(text, want) {
			return fmt.Errorf(".dockerignore does not exclude %s", want)
		}
	}
	return nil
}

func (w *World) composeService(path, service string) (map[string]any, error) {
	doc, err := w.loadYAML(path)
	if err != nil {
		return nil, err
	}
	s := asMap(asMap(doc["services"])[service])
	if s == nil {
		return nil, fmt.Errorf("%s has no %s service", path, service)
	}
	return s, nil
}

func envOf(service map[string]any) map[string]string {
	out := map[string]string{}
	switch e := service["environment"].(type) {
	case map[string]any:
		for k, v := range e {
			out[k] = fmt.Sprint(v)
		}
	case []any:
		for _, i := range e {
			kv := strings.SplitN(fmt.Sprint(i), "=", 2)
			if len(kv) == 2 {
				out[kv[0]] = kv[1]
			}
		}
	}
	return out
}

func (w *World) ThenDemoMigrateRunsTheShippedCommand(path string) error {
	s, err := w.composeService(path, "migrate")
	if err != nil {
		return err
	}
	if strings.Contains(fmt.Sprint(s["image"]), "migrate/migrate") {
		return errors.New("migrate still uses the migrate/migrate image, which cannot set the app password")
	}
	if !contains(asStrings(s["command"]), "migrate") {
		return fmt.Errorf("migrate service must run `ledgerops migrate`, command is %v", s["command"])
	}
	env := envOf(s)
	if env[string(VarAppPassword)] != DemoPassword || env[string(VarMigrateDSN)] == "" {
		return fmt.Errorf("migrate service must set %s=app-secret and %s explicitly, has %v", VarAppPassword, VarMigrateDSN, env)
	}
	return nil
}

func (w *World) ThenDemoAppNeverReceivesTheMigrationCredential(path string) error {
	s, err := w.composeService(path, "app")
	if err != nil {
		return err
	}
	if _, ok := envOf(s)[string(VarMigrateDSN)]; ok {
		return fmt.Errorf("the app service receives %s", VarMigrateDSN)
	}
	if s["env_file"] != nil {
		return errors.New("the app service uses an env_file, which could carry the migration credential")
	}
	return nil
}

func (w *World) ThenExampleComposeUsesNoDemoCredential() error {
	text, err := w.readFile(exampleComposePath)
	if err != nil {
		return err
	}
	for _, bad := range []string{DemoPassword, "demo-operator-key", DemoTenantToken} {
		if strings.Contains(text, bad) {
			return fmt.Errorf("%s contains the demo credential %q", exampleComposePath, bad)
		}
	}
	return nil
}

// ---- identity regex ------------------------------------------------------------------------

var identityRe = regexp.MustCompile(`\^https://github\.com/[^'"\s]+\$`)

func (w *World) publishedIdentity() (*regexp.Regexp, error) {
	if _, err := w.readFile(releaseWorkflowPath); err != nil {
		return nil, err
	}
	m := identityRe.FindString(w.rel.docText)
	if m == "" {
		return nil, errors.New("release.yml carries no certificate identity pattern")
	}
	return regexp.Compile(m)
}

func (w *World) ThenIdentityPatternMatches(sans []string, want bool) error {
	re, err := w.publishedIdentity()
	if err != nil {
		return err
	}
	for _, s := range sans {
		if re.MatchString(s) != want {
			return fmt.Errorf("identity %q: expected match=%v against %s", s, want, re)
		}
	}
	return nil
}

// ---- documentation ---------------------------------------------------------------------------

func (w *World) ThenDocContains(path string, texts []string) error {
	body, err := w.readFile(path)
	if err != nil {
		return err
	}
	for _, t := range texts {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(t)) {
			return fmt.Errorf("%s does not contain %q", path, t)
		}
	}
	return nil
}

func (w *World) ThenDocOmits(path string, texts []string) error {
	body, err := w.readFile(path)
	if err != nil {
		return err
	}
	for _, t := range texts {
		if strings.Contains(strings.ToLower(body), strings.ToLower(t)) {
			return fmt.Errorf("%s still contains %q", path, t)
		}
	}
	return nil
}

func (w *World) ThenDocOrders(path, first, second string) error {
	body, err := w.readFile(path)
	if err != nil {
		return err
	}
	i, j := strings.Index(body, first), strings.Index(body, second)
	if i < 0 || j < 0 || i > j {
		return fmt.Errorf("%s must mention %q before %q (positions %d, %d)", path, first, second, i, j)
	}
	return nil
}

func (w *World) ThenReadmeLeadsWithThePublishedImage() error {
	body, err := w.readFile(readmePath)
	if err != nil {
		return err
	}
	q := strings.Index(body, "Quickstart")
	pull := strings.Index(body, "docker pull ghcr.io/gizawnahom/ledgerops")
	build := strings.Index(body, "make up")
	if q < 0 || pull < 0 || (build >= 0 && pull > build) {
		return errors.New("the README quickstart must lead with the published image, before building from source")
	}
	return nil
}

// ---- promote and smoke against a local registry:2 -----------------------------------------

func (w *World) GivenLocalRegistryWithImages(ctx context.Context) error {
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "registry:2", ExposedPorts: []string{"5000/tcp"},
			WaitingFor: wait.ForListeningPort("5000/tcp").WithStartupTimeout(60 * time.Second),
		}, Started: true})
	if err != nil {
		return err
	}
	w.rel.registry = c
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5000/tcp")
	w.rel.registryAt = fmt.Sprintf("%s:%s", host, port.Port())
	w.rel.digestA, err = pushLocal(ctx, "postgres:16", w.rel.registryAt+"/gizawnahom/ledgerops:build-a")
	if err != nil {
		return err
	}
	w.rel.digestB, err = pushLocal(ctx, "node:22-slim", w.rel.registryAt+"/gizawnahom/ledgerops:build-b")
	return err
}

func pushLocal(ctx context.Context, source, target string) (string, error) {
	if out, err := exec.CommandContext(ctx, "docker", "tag", source, target).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker tag: %s", out)
	}
	out, err := exec.CommandContext(ctx, "docker", "push", target).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker push: %s", out)
	}
	m := regexp.MustCompile(`digest: (sha256:[0-9a-f]{64})`).FindStringSubmatch(string(out))
	if m == nil {
		return "", fmt.Errorf("no digest in push output: %s", out)
	}
	return m[1], nil
}

func (w *World) repo() string { return w.rel.registryAt + "/gizawnahom/ledgerops" }

func (w *World) retag(ctx context.Context, tag, digest string) error {
	out, err := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "create", "-t", w.repo()+":"+tag, w.repo()+"@"+digest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("seeding tag %s: %s", tag, out)
	}
	return nil
}

func (w *World) GivenPublishedTags(ctx context.Context, tags []string, image string) error {
	d := w.rel.digestA
	if image == "another image" {
		d = w.rel.digestB
	}
	for _, t := range tags {
		if err := w.retag(ctx, t, d); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) resolve(ctx context.Context, tag string) string {
	out, err := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", w.repo()+":"+tag, "--format", "{{.Manifest.Digest}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RunPromote executes the promote job's run steps with the DISTILL-defined
// environment contract (upstream-issues U-8).
func (w *World) RunPromote(ctx context.Context, version string, tags []string, moveLatest bool, highestStable string) error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	steps, _ := jobs["promote"]["steps"].([]any)
	var script []string
	for _, s := range steps {
		if run, ok := asMap(s)["run"].(string); ok {
			script = append(script, run)
		}
	}
	if len(script) == 0 {
		return errors.New("the promote job has no run steps")
	}
	cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", strings.Join(script, "\n"))
	cmd.Env = append(os.Environ(),
		EnvImageRepo+"="+w.repo(), EnvImageDigest+"="+w.rel.digestA,
		EnvPlanVersion+"="+version, EnvPlanTags+"="+strings.Join(tags, " "),
		fmt.Sprintf("%s=%v", EnvPlanMoveLatest, moveLatest), EnvPlanHighestStable+"="+highestStable)
	out, runErr := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		code = ee.ExitCode()
	}
	w.rel.promote = RunResult{ExitCode: code, Stdout: string(out)}
	w.outputs = append(w.outputs, string(out))
	return nil
}

func (w *World) ThenTagsResolveToTheBuiltDigest(ctx context.Context, tags []string) error {
	if w.rel.promote.ExitCode != 0 {
		return fmt.Errorf("promote failed: %s", w.rel.promote.Stdout)
	}
	for _, t := range tags {
		if got := w.resolve(ctx, t); got != w.rel.digestA {
			return fmt.Errorf("tag %s resolves to %q, expected %s", t, got, w.rel.digestA)
		}
	}
	return nil
}

func (w *World) ThenPromoteRefuses(token string) error {
	if w.rel.promote.ExitCode == 0 {
		return fmt.Errorf("expected promote to refuse, it succeeded: %s", w.rel.promote.Stdout)
	}
	if token != "" && !has(w.rel.promote.Stdout, token) {
		return fmt.Errorf("expected the refusal to say %q, got: %s", token, w.rel.promote.Stdout)
	}
	return nil
}

func (w *World) ThenTagUnchanged(ctx context.Context, tag string, image string) error {
	want := w.rel.digestA
	if image == "another image" {
		want = w.rel.digestB
	}
	if got := w.resolve(ctx, tag); got != want {
		return fmt.Errorf("tag %s moved: now %q, expected %s", tag, got, want)
	}
	return nil
}

func (w *World) ThenNoTagExists(ctx context.Context, tags []string) error {
	for _, t := range tags {
		if got := w.resolve(ctx, t); got != "" {
			return fmt.Errorf("tag %s exists (%s) but should not", t, got)
		}
	}
	return nil
}

// RunSmoke pushes the repository's own image (good) or an unrelated image
// (bad) and runs scripts/smoke against it by digest.
func (w *World) RunSmoke(ctx context.Context, good bool) error {
	if err := buildImage(ctx); err != nil {
		return err
	}
	ref := w.repo()
	digest := w.rel.digestB // unrelated image: cannot migrate
	if good {
		d, err := pushLocal(ctx, imageName, ref+":smoke-subject")
		if err != nil {
			return err
		}
		digest = d
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./scripts/smoke", "--image", ref+"@"+digest)
	cmd.Dir = repoRoot()
	out, runErr := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		code = ee.ExitCode()
	}
	w.rel.promote = RunResult{ExitCode: code, Stdout: string(out)}
	return nil
}

func (w *World) ThenSmokeReportsSeconds() error {
	if w.rel.promote.ExitCode != 0 {
		return fmt.Errorf("smoke failed (%d): %s", w.rel.promote.ExitCode, w.rel.promote.Stdout)
	}
	if !regexp.MustCompile(`smoke_pull_to_healthy_seconds=\d+`).MatchString(w.rel.promote.Stdout) {
		return fmt.Errorf("smoke printed no smoke_pull_to_healthy_seconds line: %s", w.rel.promote.Stdout)
	}
	return nil
}

// ---- @requires_external -------------------------------------------------------------------

// RequireExternal skips the scenario unless the real registry and Sigstore
// are reachable and the maintainer opted in (proved on the real v0.2.0-rc.1).
func (w *World) RequireExternal() error {
	if os.Getenv("LEDGEROPS_EXTERNAL") == "" {
		return godog.ErrSkip
	}
	return nil
}

func (w *World) cosign(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "cosign", args...).CombinedOutput()
	return string(out), err
}

const publishedIdentityRegexp = `^https://github.com/gizawNahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`

func (w *World) ThenCosignVerifies(ctx context.Context, ref string, want bool) error {
	out, err := w.cosign(ctx, "verify", ref, "--certificate-identity-regexp", publishedIdentityRegexp,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com")
	if (err == nil) != want {
		return fmt.Errorf("cosign verify %s: expected success=%v, got err=%v: %s", ref, want, err, tail(out, 300))
	}
	return nil
}

func (w *World) ThenSBOMAttestationVerifies(ctx context.Context, tag string) error {
	digest, err := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", "ghcr.io/gizawnahom/ledgerops:"+tag, "--raw").Output()
	if err != nil {
		return err
	}
	m := regexp.MustCompile(`"digest":\s*"(sha256:[0-9a-f]{64})"[^}]*"platform":\s*\{[^}]*"architecture":\s*"amd64"`).FindSubmatch(digest)
	if m == nil {
		return errors.New("no amd64 platform digest in the index")
	}
	out, verr := w.cosign(ctx, "verify-attestation", "--type", "spdxjson", "ghcr.io/gizawnahom/ledgerops@"+string(m[1]),
		"--certificate-identity-regexp", publishedIdentityRegexp, "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com")
	if verr != nil {
		return fmt.Errorf("verify-attestation failed: %s", tail(out, 300))
	}
	return nil
}

func splitWords(s string) []string { return strings.Fields(s) }

// ThenPublishedDigestUnchangedOnTheRealRegistry is proven by hand on the real
// v0.2.0-rc.1 (owner runbook): force-move the tag, re-push, compare digests.
func (w *World) ThenPublishedDigestUnchangedOnTheRealRegistry() error { return godog.ErrPending }

func (w *World) ThenBuildCoversBothArchitecturesWithLabels() error {
	with, err := w.buildStep()
	if err != nil {
		return err
	}
	platforms := strings.Split(strings.ReplaceAll(fmt.Sprint(with["platforms"]), "\n", ","), ",")
	for i := range platforms {
		platforms[i] = strings.TrimSpace(platforms[i])
	}
	sort.Strings(platforms)
	if !reflect.DeepEqual(platforms, []string{"linux/amd64", "linux/arm64"}) {
		return fmt.Errorf("build platforms must be exactly linux/amd64 and linux/arm64, got %v", platforms)
	}
	if with["labels"] == nil || !strings.Contains(fmt.Sprint(with["build-args"]), "VERSION") || !strings.Contains(fmt.Sprint(with["build-args"]), "COMMIT") {
		return fmt.Errorf("build step must pass labels and VERSION/COMMIT build args, has labels=%v build-args=%v", with["labels"], with["build-args"])
	}
	return nil
}

func (w *World) ThenReleaseTemplateNamesImageDigestAndVerify() error {
	body, err := w.readFile(".github/release-notes.md.tmpl")
	if err != nil {
		return err
	}
	for _, want := range []string{"ghcr.io/gizawnahom/ledgerops", "digest", "cosign verify", "gizawNahom"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			return fmt.Errorf("the release notes template does not mention %q", want)
		}
	}
	return nil
}

func (w *World) runScriptsOf(job string) (string, error) {
	jobs, err := w.releaseJobs()
	if err != nil {
		return "", err
	}
	steps, _ := jobs[job]["steps"].([]any)
	var scripts []string
	for _, s := range steps {
		if run, ok := asMap(s)["run"].(string); ok {
			scripts = append(scripts, run)
		}
	}
	return strings.Join(scripts, "\n"), nil
}

func (w *World) ThenReleaseJobFlagsPrereleaseAndLatest() error {
	script, err := w.runScriptsOf("github-release")
	if err != nil {
		return err
	}
	for _, want := range []string{"--prerelease", "--latest", "--verify-tag", "--generate-notes"} {
		if !strings.Contains(script, want) {
			return fmt.Errorf("github-release scripts lack %s", want)
		}
	}
	return nil
}

func (w *World) ThenSigningFailureStopsTheRelease() error {
	jobs, err := w.releaseJobs()
	if err != nil {
		return err
	}
	sign, ok := jobs["sign"]
	if !ok {
		return errors.New("release.yml has no sign job")
	}
	if fmt.Sprint(sign["continue-on-error"]) == "true" {
		return errors.New("the sign job must not continue on error")
	}
	steps, _ := sign["steps"].([]any)
	for _, s := range steps {
		if fmt.Sprint(asMap(s)["continue-on-error"]) == "true" {
			return errors.New("a sign step continues on error, so a signing failure would not fail the release")
		}
	}
	for _, downstream := range []string{"anon-probe", "promote"} {
		if !strings.Contains(fmt.Sprint(jobs[downstream]["if"]), "needs.sign.result == 'success'") {
			return fmt.Errorf("%s does not require the sign job to have succeeded", downstream)
		}
	}
	return nil
}

func (w *World) ThenIndexListsBothArchitecturesWithLabels(ctx context.Context, tag string) error {
	ref := "ghcr.io/gizawnahom/ledgerops:" + tag
	out, err := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", ref).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "linux/amd64") || !strings.Contains(string(out), "linux/arm64") {
		return fmt.Errorf("index of %s does not list both architectures: %s", ref, tail(string(out), 300))
	}
	if o, err := exec.CommandContext(ctx, "docker", "pull", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("pull: %s", o)
	}
	label, _ := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{ index .Config.Labels "org.opencontainers.image.version" }}`, ref).Output()
	if strings.TrimSpace(string(label)) != tag {
		return fmt.Errorf("version label is %q, expected %q", strings.TrimSpace(string(label)), tag)
	}
	return nil
}

func (w *World) ThenQuickstartGeneratesSecretsAndWarns() error {
	return w.ThenDocContains(quickstartPath, []string{"openssl rand", "LEDGEROPS_APP_PASSWORD_FILE", "app-secret", "docker inspect"})
}

func (w *World) ThenQuickstartWarnsAboutStatementLogging() error {
	return w.ThenDocContains(quickstartPath, []string{"log_statement", "offline"})
}

func (w *World) ThenQuickstartOrdersMigrateBeforeRestart() error {
	if err := w.ThenDocOrders(quickstartPath, "migrate", "restart"); err != nil {
		return err
	}
	return w.ThenDocContains(quickstartPath, []string{"restart"})
}

func (w *World) ThenQuickstartShowsVerification() error {
	return w.ThenDocContains(quickstartPath, []string{"cosign verify", "verify-attestation", "gizawNahom", "ghcr.io/gizawnahom/ledgerops"})
}

func (w *World) ThenQuickstartExplainsInterruptedMigrationRecovery() error {
	return w.ThenDocContains(quickstartPath, []string{"schema_migrations", "dirty = false"})
}

func (w *World) ThenQuickstartStatesOneDatabasePerCluster() error {
	return w.ThenDocContains(quickstartPath, []string{"DROP ROLE ledgerops_app", "one ledgerops database per", "cluster"})
}

func (w *World) ThenQuickstartRecommendsDigestPinning() error {
	return w.ThenDocContains(quickstartPath, []string{"pin", "digest", "tag immutability"})
}

// The documented commands are run verbatim by the release's quickstart job
// against the real release; deferred here by decision (see DISTILL section).
func (w *World) WhenQuickstartCommandsRunAgainstTheRelease() error { return godog.ErrPending }
func (w *World) ThenQuickstartTransferAnswered() error             { return godog.ErrPending }
func (w *World) WhenTagMovedAndPushedOnTheRealRegistry() error     { return godog.ErrPending }

func (w *World) ThenSmokeFailsAtTheMigrationStep() error {
	if w.rel.promote.ExitCode == 0 {
		return fmt.Errorf("expected the smoke check to fail, it succeeded: %s", w.rel.promote.Stdout)
	}
	if !has(w.rel.promote.Stdout, TokenMigrateStep.String()) || has(w.rel.promote.Stdout, "__SCAFFOLD__") {
		return fmt.Errorf("expected the smoke failure to name the %s step, got: %s", TokenMigrateStep, w.rel.promote.Stdout)
	}
	return nil
}

func (w *World) ThenNoVersionTagOnTheRegistry(ctx context.Context, version string) error {
	return w.ThenNoTagExists(ctx, []string{version})
}
