package containerimagerelease

// serve, version and console driving ports of the World.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	dsnSentinelPassword  = "dsn-sentinel-5c1e"
	migrateSentinelDSN   = "postgres://ledgerops_migrate:migrate-sentinel-77@db.invalid:5432/ledgerops?sslmode=disable"
	consoleBundleSubpath = "internal/adapters/http/console_dist"
)

func (w *World) GivenServeSetting(v Variable, state ValueState) error {
	if state == HasValue {
		delete(w.serveOverrides, v)
		return nil
	}
	w.serveOverrides[v] = envOverride{state: state, value: state.Literal()}
	return nil
}

func (w *World) GivenServeValue(v Variable, value string) error {
	w.serveOverrides[v] = envOverride{state: HasValue, value: value}
	w.trackSecret(value)
	return nil
}

func (w *World) GivenMigrationCredentialPresent() error {
	w.serveOverrides[VarMigrateDSN] = envOverride{state: HasValue, value: migrateSentinelDSN}
	w.trackSecret("migrate-sentinel-77")
	return nil
}

func (w *World) GivenNoDatabaseReachable() error {
	t, err := newTripwire()
	if err != nil {
		return err
	}
	w.tripwire = t
	w.trackSecret(dsnSentinelPassword)
	return nil
}

func (w *World) GivenAppLoginPassword(password string) error {
	w.dsnPassword = password
	w.trackSecret(password)
	return nil
}

func (w *World) defaultAppDSN() string {
	host := "127.0.0.1"
	if w.useImage {
		host = "host.docker.internal"
	}
	if w.tripwire != nil {
		return fmt.Sprintf("postgres://ledgerops_app:%s@%s:%d/ledgerops?sslmode=disable", dsnSentinelPassword, host, w.tripwire.Port())
	}
	pw := w.dsnPassword
	if pw == "" {
		pw = w.password
	}
	if pw == "" {
		pw = ChosenPassword
	}
	return w.cluster.DSN(string(w.target), appRole, pw, w.useImage)
}

func (w *World) serveInvocation() (Invocation, int, error) {
	env := map[string]string{}
	port := 8080
	if !w.useImage {
		p, err := freePort()
		if err != nil {
			return Invocation{}, 0, err
		}
		port = p
		env[string(VarAddr)] = fmt.Sprintf("127.0.0.1:%d", port)
	} else {
		p, err := freePort()
		if err != nil {
			return Invocation{}, 0, err
		}
		port = p
		env[string(VarAddr)] = ":8080"
	}
	defaults := map[Variable]*string{}
	dsn := w.defaultAppDSN()
	op := OperatorKeyValue
	defaults[VarAppDSN] = &dsn
	defaults[VarOperatorKey] = &op
	w.trackSecret(OperatorKeyValue)
	for _, v := range []Variable{VarAppDSN, VarOperatorKey, VarDemoTenantKey, VarMigrateDSN} {
		if o, ok := w.serveOverrides[v]; ok {
			switch o.state {
			case Unset:
			default:
				env[string(v)] = o.value
			}
			continue
		}
		if d := defaults[v]; d != nil {
			env[string(v)] = *d
		}
	}
	inv := Invocation{Env: env, Variant: w.variant, Dir: w.consoleCwd, Timeout: 30 * time.Second}
	if w.useImage {
		inv.Port = port
	}
	return inv, port, nil
}

// StartLedgerops starts the default (serve) command and waits until it either
// listens or exits.
func (w *World) StartLedgerops(ctx context.Context) error {
	inv, port, err := w.serveInvocation()
	if err != nil {
		return err
	}
	proc, err := w.runner.Start(ctx, inv)
	if err != nil {
		return err
	}
	w.serveProc = proc
	w.serveURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	w.serveListening = waitForPort(proc, port, 25*time.Second)
	if !w.serveListening && !proc.Exited() {
		// neither listening nor exited within the window: treat as running
		w.serveListening = false
	}
	return nil
}

func (w *World) ThenRefusesToStart(tokens []string) error {
	if w.serveProc == nil {
		return errors.New("ledgerops was never started")
	}
	res := w.serveProc.Wait(8 * time.Second)
	if res.TimedOut {
		return fmt.Errorf("expected ledgerops to refuse to start, but it kept running; output: %s", res.Output())
	}
	if res.ExitCode != 1 {
		return fmt.Errorf("expected exit 1, got %d; output: %s", res.ExitCode, res.Output())
	}
	for _, t := range tokens {
		if !has(res.Output(), t) {
			return fmt.Errorf("expected the refusal to name %q, got: %s", t, res.Output())
		}
	}
	return nil
}

func (w *World) ThenNoDatabaseConnectionAttempted() error {
	if w.tripwire == nil {
		return errors.New("scenario error: no unreachable database was arranged")
	}
	if n := w.tripwire.Attempts(); n != 0 {
		return fmt.Errorf("ledgerops tried to reach the database %d time(s) before refusing", n)
	}
	return nil
}

func (w *World) get(ctx context.Context, path, bearer string) (int, string, error) {
	h := map[string]string{}
	if bearer != "" {
		h["Authorization"] = "Bearer " + bearer
	}
	status, body, err := httpDo(ctx, "GET", w.serveURL+path, h, nil)
	w.lastStatus, w.lastBody = status, body
	return status, body, err
}

func (w *World) post(ctx context.Context, path, bearer string, payload any, key string) (int, string, error) {
	h := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + bearer}
	if key != "" {
		h["Idempotency-Key"] = key
	}
	status, body, err := httpDo(ctx, "POST", w.serveURL+path, h, jsonBody(payload))
	w.lastStatus, w.lastBody = status, body
	return status, body, err
}

func (w *World) mustBeServing() error {
	if w.serveProc == nil || !w.serveListening {
		out := ""
		if w.serveProc != nil {
			out = w.serveProc.Result().Output()
		}
		return fmt.Errorf("expected ledgerops to be serving, but it is not (output: %s)", out)
	}
	return nil
}

func (w *World) ThenReportsHealthy(ctx context.Context) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, _, err := w.get(ctx, "/health", "")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("expected GET /health to answer 200 without credentials, got %d", status)
	}
	return nil
}

func (w *World) ThenHealthIsBare(ctx context.Context) error {
	status, body, err := w.get(ctx, "/health", "")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("expected 200, got %d", status)
	}
	for _, leak := range []string{"entries", "balance", "tenant", "account", OperatorKeyValue} {
		if strings.Contains(strings.ToLower(body), leak) {
			return fmt.Errorf("the health answer carries ledger data (%q): %s", leak, body)
		}
	}
	return nil
}

func (w *World) ThenTokenRefused(ctx context.Context, token string) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, _, err := w.get(ctx, "/accounts/probe-account", token)
	if err != nil {
		return err
	}
	if status != 401 {
		return fmt.Errorf("expected the token %q to be refused 401, got %d", token, status)
	}
	return nil
}

func (w *World) ThenTokenResolvesToTheLegacyTenant(ctx context.Context, token string) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, body, err := w.post(ctx, "/accounts", token, map[string]string{"account_id": "demo-probe", "type": "wallet"}, "")
	if err != nil {
		return err
	}
	if status != 201 && status != 200 {
		return fmt.Errorf("expected the demo token to open an account as the legacy tenant, got %d: %s", status, body)
	}
	return nil
}

func (w *World) ThenOperatorKeyAccepted(ctx context.Context) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, body, err := w.post(ctx, "/tenants", OperatorKeyValue, map[string]string{"name": "Kestrel Probe"}, "")
	if err != nil {
		return err
	}
	if status != 201 {
		return fmt.Errorf("expected the operator key to be accepted on /tenants (201), got %d: %s", status, body)
	}
	return nil
}

func (w *World) PostFirstTransfer(ctx context.Context, from, to string) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, body, err := w.post(ctx, "/tenants", OperatorKeyValue, map[string]string{"name": "Kestrel Wallet"}, "")
	if err != nil || status != 201 {
		return fmt.Errorf("provisioning a tenant failed: %d %s %v", status, body, err)
	}
	var tenant struct {
		Key string `json:"tenant_key"`
	}
	_ = json.Unmarshal([]byte(body), &tenant)
	w.tenantKey = tenant.Key
	for _, a := range []struct{ id, kind string }{{from, "system"}, {to, "wallet"}} {
		if s, b, e := w.post(ctx, "/accounts", w.tenantKey, map[string]string{"account_id": a.id, "type": a.kind}, ""); e != nil || s != 201 {
			return fmt.Errorf("opening %s failed: %d %s %v", a.id, s, b, e)
		}
	}
	status, body, err = w.post(ctx, "/transfers", w.tenantKey, map[string]string{"from": from, "to": to, "amount": "10.00"}, "first-transfer")
	if err != nil || status != 201 && status != 200 {
		return fmt.Errorf("the first transfer was not posted: %d %s %v", status, body, err)
	}
	var posted struct {
		ID string `json:"transaction_id"`
	}
	_ = json.Unmarshal([]byte(body), &posted)
	w.transactionID = posted.ID
	w.lastBody = body
	return nil
}

func (w *World) ThenTransferAnsweredWithATransactionID() error {
	if w.transactionID == "" {
		return fmt.Errorf("the transfer answer carries no transaction id: %s", w.lastBody)
	}
	return nil
}

func (w *World) ThenNoLogLineContains(secret string) error {
	if w.serveProc == nil {
		return errors.New("ledgerops was never started")
	}
	w.serveProc.Wait(500 * time.Millisecond)
	if out := w.serveProc.Result().Output(); strings.Contains(out, secret) {
		return fmt.Errorf("%q appears in ledgerops output: %s", secret, out)
	}
	return nil
}

// ---- other commands --------------------------------------------------------------------

func (w *World) RunCommand(ctx context.Context, args ...string) error {
	res, err := w.runner.Run(ctx, Invocation{Args: args, Env: map[string]string{}, Variant: w.variant, Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	w.record(res)
	return nil
}

func (w *World) ThenCommandRefusesNaming(tokens []string) error {
	if w.last.ExitCode == 0 {
		return fmt.Errorf("expected a non-zero exit, got 0: %s", w.last.Output())
	}
	for _, t := range tokens {
		if !has(w.last.Output(), t) {
			return fmt.Errorf("expected the usage message to name %q, got: %s", t, w.last.Output())
		}
	}
	return nil
}

func (w *World) GivenBinaryBuiltFor(version, commit string) error {
	w.variant = BinaryVariant{Version: version, Commit: commit}
	return nil
}

func (w *World) GivenBinaryBuiltWithoutVersion() error {
	w.variant = BinaryVariant{}
	return nil
}

func (w *World) ThenItPrints(text string) error {
	if w.last.ExitCode != 0 || !strings.Contains(w.last.Output(), text) {
		return fmt.Errorf("expected exit 0 printing %q, got exit %d: %s", text, w.last.ExitCode, w.last.Output())
	}
	return nil
}

// ---- console bundle ---------------------------------------------------------------------

func (w *World) GivenConsoleBundle(state ConsoleBundle) error {
	w.bundle = state
	return nil
}

// WhenBuiltForTheImage compiles with the console_embed tag against a virtual
// console_dist (go build -overlay), leaving the working tree untouched.
func (w *World) WhenBuiltForTheImage(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "ledgerops-overlay-")
	if err != nil {
		return err
	}
	registerTempDir(dir)
	replace := map[string]string{}
	stage := func(name, contents string) error {
		src := filepath.Join(dir, "stage-"+name)
		if err := os.WriteFile(src, []byte(contents), 0o644); err != nil {
			return err
		}
		replace[filepath.Join(repoRoot(), consoleBundleSubpath, name)] = src
		return nil
	}
	switch w.bundle {
	case BundleNoIndex:
		err = stage("placeholder.txt", "not the console")
	case BundlePresent:
		err = stage("index.html", "<!doctype html><title>ledgerops console</title>")
	}
	if err != nil {
		return err
	}
	overlay, _ := json.Marshal(map[string]any{"Replace": replace})
	if err := os.WriteFile(filepath.Join(dir, "overlay.json"), overlay, 0o644); err != nil {
		return err
	}
	w.variant = BinaryVariant{Tags: "console_embed", OverlayDir: dir}
	_, w.buildErr = TryBuild(ctx, w.variant)
	return nil
}

func (w *World) ThenBuildFails() error {
	if w.buildErr == nil {
		return errors.New("expected the build to fail without a console bundle, but it produced a binary")
	}
	return nil
}

func (w *World) ThenBuildSucceeds() error {
	return w.buildErr
}

// GivenUntaggedBuildWithBundleInWorkingDirectory prepares the dev-mode path:
// no embed tag, bundle found relative to the working directory.
func (w *World) GivenUntaggedBuildWithBundleInWorkingDirectory() error {
	cwd := filepath.Join(w.tmpDir, "cwd")
	dist := filepath.Join(cwd, "web", "console", "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!doctype html><title>dev console</title>"), 0o644); err != nil {
		return err
	}
	w.variant = BinaryVariant{}
	w.consoleCwd = cwd
	return nil
}

func (w *World) GivenEmptyWorkingDirectory() error {
	cwd := filepath.Join(w.tmpDir, "emptycwd")
	w.consoleCwd = cwd
	return os.MkdirAll(cwd, 0o755)
}

func (w *World) ThenConsolePageIsReturned(ctx context.Context) error {
	if err := w.mustBeServing(); err != nil {
		return err
	}
	status, body, err := w.get(ctx, "/console", "")
	if err != nil {
		return err
	}
	if status != 200 || !strings.Contains(strings.ToLower(body), "<!doctype html") && !strings.Contains(strings.ToLower(body), "<html") {
		return fmt.Errorf("expected the console page (200 with HTML), got %d: %s", status, tail(body, 200))
	}
	return nil
}

func (w *World) ThenVerdictNeedsTheOperatorKey(ctx context.Context) error {
	if status, _, err := w.get(ctx, "/console/verdict", ""); err != nil || status != 401 {
		return fmt.Errorf("expected 401 without a key, got %d (%v)", status, err)
	}
	if status, body, err := w.get(ctx, "/console/verdict", OperatorKeyValue); err != nil || status != 200 {
		return fmt.Errorf("expected the verdict with the operator key, got %d %s (%v)", status, body, err)
	}
	return nil
}
