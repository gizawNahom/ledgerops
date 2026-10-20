package containerimagerelease

// Process harness: builds the ledgerops binary / image once per test package
// and runs them as subprocesses (policy rows "ledgerops binary" and
// "Container image", 2026-10-19). Nothing here knows about Postgres.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// repoRoot is the module root, found from this file's location.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// ---- build once ------------------------------------------------------------

type buildResult struct {
	path string
	err  error
}

var (
	buildMu    sync.Mutex
	builds     = map[string]*buildResult{}
	buildDir   string
	imageOnce  sync.Once
	imageErr   error
	imageName  = "ledgerops-acceptance:dev"
	cleanupDir []string
)

// BinaryVariant selects how the host binary is built.
type BinaryVariant struct {
	Tags       string // go build -tags
	Version    string // -X main.version
	Commit     string // -X main.commit
	OverlayDir string // a directory holding overlay.json, "" for none
}

func (v BinaryVariant) key() string {
	return strings.Join([]string{v.Tags, v.Version, v.Commit, v.OverlayDir}, "|")
}

// hostBinary builds ./cmd/api once per distinct variant and returns the path.
func hostBinary(ctx context.Context, v BinaryVariant) (string, error) {
	buildMu.Lock()
	defer buildMu.Unlock()
	if r, ok := builds[v.key()]; ok {
		return r.path, r.err
	}
	if buildDir == "" {
		dir, err := os.MkdirTemp("", "ledgerops-acceptance-bin-")
		if err != nil {
			return "", err
		}
		buildDir = dir
	}
	out := filepath.Join(buildDir, fmt.Sprintf("ledgerops-%d", len(builds)))
	args := []string{"build", "-o", out}
	if v.Tags != "" {
		args = append(args, "-tags", v.Tags)
	}
	var ldflags []string
	if v.Version != "" {
		ldflags = append(ldflags, "-X main.version="+v.Version)
	}
	if v.Commit != "" {
		ldflags = append(ldflags, "-X main.commit="+v.Commit)
	}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	if v.OverlayDir != "" {
		args = append(args, "-overlay", filepath.Join(v.OverlayDir, "overlay.json"))
	}
	args = append(args, "./cmd/api")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoRoot()
	output, err := cmd.CombinedOutput()
	r := &buildResult{path: out}
	if err != nil {
		r.err = &BuildFailure{Output: string(output)}
	}
	builds[v.key()] = r
	return r.path, r.err
}

// BuildFailure is a `go build` or `docker build` that failed; scenarios that
// expect a failing build read Output.
type BuildFailure struct{ Output string }

func (b *BuildFailure) Error() string { return "build failed: " + b.Output }

// TryBuild builds a variant uncached and reports the failure instead of
// returning it as a harness error (for "the build fails" scenarios).
func TryBuild(ctx context.Context, v BinaryVariant) (string, error) {
	buildMu.Lock()
	delete(builds, v.key())
	buildMu.Unlock()
	return hostBinary(ctx, v)
}

// buildImage builds the repository Dockerfile once per package.
func buildImage(ctx context.Context) error {
	imageOnce.Do(func() {
		cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageName, ".")
		cmd.Dir = repoRoot()
		out, err := cmd.CombinedOutput()
		if err != nil {
			imageErr = &BuildFailure{Output: tail(string(out), 4000)}
		}
	})
	return imageErr
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// CleanupBuilds removes build artefacts (AfterSuite).
func CleanupBuilds() {
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	for _, d := range cleanupDir {
		_ = os.RemoveAll(d)
	}
}

func registerTempDir(dir string) {
	buildMu.Lock()
	cleanupDir = append(cleanupDir, dir)
	buildMu.Unlock()
}

// ---- running ---------------------------------------------------------------

// Invocation is one run of the ledgerops binary or image.
type Invocation struct {
	Args  []string
	Env   map[string]string
	Files []MountedFile
	Dir   string // working directory (host binary only)
	// Variant selects a non-default host binary build.
	Variant BinaryVariant
	Timeout time.Duration
	// Port, when non-zero on an image run, publishes container port 8080 there.
	Port int
}

// MountedFile is a secret file made available to the process. For the image
// it is bind-mounted read-only at ContainerPath.
type MountedFile struct {
	HostPath      string
	ContainerPath string
}

// RunResult is what a finished (or abandoned) run left behind.
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Took     time.Duration
}

// Output is stdout and stderr together.
func (r RunResult) Output() string { return r.Stdout + "\n" + r.Stderr }

// Runner runs the driving port.
type Runner interface {
	Run(ctx context.Context, inv Invocation) (RunResult, error)
	Start(ctx context.Context, inv Invocation) (*Process, error)
}

// hostRunner executes the built binary.
type hostRunner struct{}

// imageRunner executes the built image with docker run.
type imageRunner struct{}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys)+1)
	out = append(out, "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func (hostRunner) command(ctx context.Context, inv Invocation) (*exec.Cmd, error) {
	bin, err := hostBinary(ctx, inv.Variant)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, inv.Args...)
	cmd.Env = envList(inv.Env)
	cmd.Dir = inv.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

func (imageRunner) command(ctx context.Context, inv Invocation, name string) (*exec.Cmd, error) {
	if err := buildImage(ctx); err != nil {
		return nil, err
	}
	args := []string{"run", "--rm", "--name", name, "--add-host=host.docker.internal:host-gateway"}
	keys := make([]string, 0, len(inv.Env))
	for k := range inv.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+inv.Env[k])
	}
	for _, f := range inv.Files {
		args = append(args, "-v", f.HostPath+":"+f.ContainerPath+":ro")
	}
	if inv.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:8080", inv.Port))
	}
	args = append(args, imageName)
	args = append(args, inv.Args...)
	return exec.CommandContext(ctx, "docker", args...), nil
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Process is a started, possibly still running, run.
type Process struct {
	cmd    *exec.Cmd
	out    *lockedBuffer
	errBuf *lockedBuffer
	done   chan struct{}
	exit   int
	name   string
	image  bool
	begin  time.Time
}

func newProcess(cmd *exec.Cmd, name string, image bool) (*Process, error) {
	p := &Process{cmd: cmd, out: &lockedBuffer{}, errBuf: &lockedBuffer{}, done: make(chan struct{}), name: name, image: image, begin: time.Now()}
	cmd.Stdout = p.out
	cmd.Stderr = p.errBuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		err := cmd.Wait()
		p.exit = 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			p.exit = ee.ExitCode()
		} else if err != nil {
			p.exit = -1
		}
		close(p.done)
	}()
	return p, nil
}

// Exited reports whether the process has finished.
func (p *Process) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Result snapshots output so far.
func (p *Process) Result() RunResult {
	r := RunResult{Stdout: p.out.String(), Stderr: p.errBuf.String(), Took: time.Since(p.begin)}
	if p.Exited() {
		r.ExitCode = p.exit
	} else {
		r.ExitCode = -1
	}
	return r
}

// Wait blocks until exit or the timeout; the result's TimedOut says which.
func (p *Process) Wait(timeout time.Duration) RunResult {
	select {
	case <-p.done:
	case <-time.After(timeout):
		r := p.Result()
		r.TimedOut = true
		return r
	}
	return p.Result()
}

// Kill terminates the process (SIGKILL), and removes the container for an image run.
func (p *Process) Kill() {
	if p.image {
		_ = exec.Command("docker", "rm", "-f", p.name).Run()
	} else if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}

var containerSeq int
var containerMu sync.Mutex

func nextContainerName() string {
	containerMu.Lock()
	defer containerMu.Unlock()
	containerSeq++
	return fmt.Sprintf("ledgerops-at-%d-%d", os.Getpid(), containerSeq)
}

func (r hostRunner) Start(ctx context.Context, inv Invocation) (*Process, error) {
	cmd, err := r.command(context.WithoutCancel(ctx), inv)
	if err != nil {
		return nil, err
	}
	return newProcess(cmd, "", false)
}

func (r imageRunner) Start(ctx context.Context, inv Invocation) (*Process, error) {
	name := nextContainerName()
	cmd, err := r.command(context.WithoutCancel(ctx), inv, name)
	if err != nil {
		return nil, err
	}
	return newProcess(cmd, name, true)
}

func runToEnd(p *Process, err error, timeout time.Duration) (RunResult, error) {
	if err != nil {
		return RunResult{}, err
	}
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	res := p.Wait(timeout)
	if res.TimedOut {
		p.Kill()
		res = p.Result()
		res.TimedOut = true
	}
	return res, nil
}

func (r hostRunner) Run(ctx context.Context, inv Invocation) (RunResult, error) {
	p, err := r.Start(ctx, inv)
	return runToEnd(p, err, inv.Timeout)
}

func (r imageRunner) Run(ctx context.Context, inv Invocation) (RunResult, error) {
	p, err := r.Start(ctx, inv)
	return runToEnd(p, err, inv.Timeout)
}

// ---- small network helpers ---------------------------------------------------

// freePort asks the OS for an unused TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForPort returns when something accepts on the port, the process exits,
// or the timeout elapses.
func waitForPort(p *Process, port int, timeout time.Duration) (listening bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.Exited() {
			return false
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// httpDo performs one request and returns status and body.
func httpDo(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String(), nil
}

// Tripwire is a TCP listener standing in for "the database": any connection
// attempt is counted, proving whether ledgerops tried to reach it.
type Tripwire struct {
	l        net.Listener
	mu       sync.Mutex
	attempts int
}

func newTripwire() (*Tripwire, error) {
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	t := &Tripwire{l: l}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.mu.Lock()
			t.attempts++
			t.mu.Unlock()
			c.Close()
		}
	}()
	return t, nil
}

func (t *Tripwire) Port() int { return t.l.Addr().(*net.TCPAddr).Port }
func (t *Tripwire) Attempts() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attempts
}
func (t *Tripwire) Close() { _ = t.l.Close() }
