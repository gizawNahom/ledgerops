// console_static_test.go exercises mountConsole's two states directly:
// web/console/dist absent (today, before ledger-core-console's DELIVER
// wave) and present (after). Both states are constructed with t.TempDir and
// os.Chdir rather than depending on the repository's actual working tree,
// so this test is correct on a fresh clone before web/console/ ever exists.
package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/app/ports"
)

// fakeStore is the minimal ports.Store this package's router needs to build
// -- no handler exercised here reads or writes through it, so every method
// is unreachable and panics if called.
type fakeStore struct{ ports.Store }

func newTestDeps() apphttp.Deps {
	return apphttp.Deps{
		Store:       fakeStore{},
		OperatorKey: "test-operator-key",
		Clock:       func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) },
		IDGenerator: func() string { return "txn_test" },
	}
}

// chdir moves the process working directory to dir for the duration of the
// test, restoring it on cleanup. mountConsole resolves consoleDistDir
// relative to the working directory, so this is how the test controls
// whether "web/console/dist" resolves to something that exists.
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restoring working directory to %q: %v", original, err)
		}
	})
}

// TestConsoleRoute_DistAbsent_NoRouteRegistered pins the "before DELIVER
// lands" state: web/console/dist does not exist, so GET /console must 404
// through chi's ordinary unmatched-route handling rather than the server
// failing to start or panicking.
func TestConsoleRoute_DistAbsent_NoRouteRegistered(t *testing.T) {
	chdir(t, t.TempDir())

	handler := apphttp.NewRouter(newTestDeps())
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/console")
	if err != nil {
		t.Fatalf("GET /console: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (dist absent)", resp.StatusCode, http.StatusNotFound)
	}
}

// TestConsoleRoute_DistPresent_ServesShellWithoutOperatorKey pins the "after
// DELIVER lands" state: web/console/dist/index.html exists, GET /console
// serves it, and -- the load-bearing assertion -- no Authorization header is
// required, because the HTML shell is not a ledger operation.
func TestConsoleRoute_DistPresent_ServesShellWithoutOperatorKey(t *testing.T) {
	root := t.TempDir()
	distDir := filepath.Join(root, "web", "console", "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const shellHTML = "<!doctype html><title>ledgerops console</title>"
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte(shellHTML), 0o644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}
	const assetJS = "console.assert(true)"
	if err := os.WriteFile(filepath.Join(distDir, "assets", "index-abc123.js"), []byte(assetJS), 0o644); err != nil {
		t.Fatalf("writing asset: %v", err)
	}

	chdir(t, root)

	handler := apphttp.NewRouter(newTestDeps())
	server := httptest.NewServer(handler)
	defer server.Close()

	t.Run("GET /console serves the shell with no Authorization header", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/console")
		if err != nil {
			t.Fatalf("GET /console: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(body) != shellHTML {
			t.Fatalf("body = %q, want %q", body, shellHTML)
		}
	})

	t.Run("GET /console/assets/* serves the built asset with no Authorization header", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/console/assets/index-abc123.js")
		if err != nil {
			t.Fatalf("GET /console/assets/index-abc123.js: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(body) != assetJS {
			t.Fatalf("body = %q, want %q", body, assetJS)
		}
	})

	t.Run("GET /console/verdict still requires the operator key", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/console/verdict")
		if err != nil {
			t.Fatalf("GET /console/verdict: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d (unauthenticated, no key presented)", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}
