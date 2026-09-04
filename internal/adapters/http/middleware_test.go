// middleware_test.go is the paired unit-test contract for step 02-03's
// shared credential-mechanism primitives (DDD-22): bearerToken/isOperatorKey
// factored out of requireOperatorKey, and the two new middlewares built on
// top of them. White-box (package http, not http_test) because it targets
// unexported middleware constructors directly -- the same convention
// handlers_test.go already uses for hashIdempotencyKey.
//
// requireTenantKey and requireTenantKeyOrOperatorKey are exercised here with
// a fake ports.TenantKeyResolver (a plain function literal -- the port is a
// function type, so a fake is a one-line value, no stub type needed) rather
// than a real PostgreSQL-backed resolver: this is pure middleware logic
// (context injection and refusal-shape), already proven against a real
// database by internal/adapters/postgres's own TenantKeyResolver tests
// (step 01-03) and by the walking-skeleton acceptance suite that will wire
// this middleware end-to-end (step 02-04).
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		name      string
		header    string
		setHeader bool
		wantToken string
		wantOK    bool
	}{
		{name: "no Authorization header at all", setHeader: false, wantToken: "", wantOK: false},
		{name: "wrong scheme", header: "Basic dXNlcjpwYXNz", setHeader: true, wantToken: "", wantOK: false},
		{name: "well-formed bearer token", header: "Bearer a-tenant-key", setHeader: true, wantToken: "a-tenant-key", wantOK: true},
		{name: "bearer prefix with empty token", header: "Bearer ", setHeader: true, wantToken: "", wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.setHeader {
				r.Header.Set("Authorization", tc.header)
			}
			gotToken, gotOK := bearerToken(r)
			if gotToken != tc.wantToken || gotOK != tc.wantOK {
				t.Fatalf("bearerToken() = (%q, %v), want (%q, %v)", gotToken, gotOK, tc.wantToken, tc.wantOK)
			}
		})
	}
}

func TestIsOperatorKey(t *testing.T) {
	cases := []struct {
		name                string
		presented, expected string
		want                bool
	}{
		{name: "exact match", presented: "the-operator-key", expected: "the-operator-key", want: true},
		{name: "mismatch", presented: "wrong-key", expected: "the-operator-key", want: false},
		{name: "empty expected never matches, even an empty presented", presented: "", expected: "", want: false},
		{name: "empty expected never matches a non-empty presented", presented: "anything", expected: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOperatorKey(tc.presented, tc.expected); got != tc.want {
				t.Fatalf("isOperatorKey(%q, %q) = %v, want %v", tc.presented, tc.expected, got, tc.want)
			}
		})
	}
}

// --- requireOperatorKey: unchanged behavior after the DDD-22 extraction ----

func TestRequireOperatorKey_UnchangedRefusalAndSuccessShape(t *testing.T) {
	const operatorKey = "the-operator-key"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := requireOperatorKey(operatorKey)(next)

	t.Run("missing Authorization header refuses 401 unidentified_caller", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assertUnidentifiedCallerRefusal(t, w)
	})

	t.Run("wrong key refuses 401 unidentified_caller", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer wrong-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		assertUnidentifiedCallerRefusal(t, w)
	})

	t.Run("empty configured operator key refuses every caller", func(t *testing.T) {
		emptyKeyHandler := requireOperatorKey("")(next)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer ")
		w := httptest.NewRecorder()
		emptyKeyHandler.ServeHTTP(w, r)
		assertUnidentifiedCallerRefusal(t, w)
	})

	t.Run("correct key is accepted", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+operatorKey)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
	})
}

// --- requireTenantKey -------------------------------------------------

func TestRequireTenantKey(t *testing.T) {
	const knownTenantID = "tnt_acme"

	resolve := func(_ context.Context, hash string) (string, bool, error) {
		if hash == hashBearerToken("known-token") {
			return knownTenantID, true, nil
		}
		return "", false, nil
	}

	var sawScope tenantScopeProbe
	next := sawScope.handler()
	handler := requireTenantKey(resolve)(next)

	t.Run("missing bearer token refuses 401 unidentified_caller", func(t *testing.T) {
		sawScope.reset()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts", nil))
		assertUnidentifiedCallerRefusal(t, w)
		if sawScope.called {
			t.Fatalf("expected next handler not to run on refusal")
		}
	})

	t.Run("unresolved token refuses 401 unidentified_caller", func(t *testing.T) {
		sawScope.reset()
		r := httptest.NewRequest(http.MethodPost, "/accounts", nil)
		r.Header.Set("Authorization", "Bearer never-issued-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		assertUnidentifiedCallerRefusal(t, w)
		if sawScope.called {
			t.Fatalf("expected next handler not to run on refusal")
		}
	})

	t.Run("resolver error refuses 401 unidentified_caller", func(t *testing.T) {
		sawScope.reset()
		erroringHandler := requireTenantKey(func(context.Context, string) (string, bool, error) {
			return "", false, errBoom
		})(next)
		r := httptest.NewRequest(http.MethodPost, "/accounts", nil)
		r.Header.Set("Authorization", "Bearer known-token")
		w := httptest.NewRecorder()
		erroringHandler.ServeHTTP(w, r)
		assertUnidentifiedCallerRefusal(t, w)
		if sawScope.called {
			t.Fatalf("expected next handler not to run on refusal")
		}
	})

	t.Run("valid tenant key injects the resolved tenant_id and proceeds", func(t *testing.T) {
		sawScope.reset()
		r := httptest.NewRequest(http.MethodPost, "/accounts", nil)
		r.Header.Set("Authorization", "Bearer known-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		if !sawScope.called {
			t.Fatalf("expected next handler to run")
		}
		gotTenantID, scoped := sawScope.scope.Resolve()
		if !scoped || gotTenantID != knownTenantID {
			t.Fatalf("expected scoped tenant_id %q, got scoped=%v tenant_id=%q", knownTenantID, scoped, gotTenantID)
		}
	})
}

// --- requireTenantKeyOrOperatorKey -------------------------------------

func TestRequireTenantKeyOrOperatorKey(t *testing.T) {
	const operatorKey = "the-operator-key"
	const knownTenantID = "tnt_acme"

	resolve := func(_ context.Context, hash string) (string, bool, error) {
		if hash == hashBearerToken("known-tenant-key") {
			return knownTenantID, true, nil
		}
		return "", false, nil
	}

	var sawScope tenantScopeProbe
	next := sawScope.handler()
	handler := requireTenantKeyOrOperatorKey(operatorKey, resolve)(next)

	t.Run("operator key is accepted byte-identically and marks the scope unscoped", func(t *testing.T) {
		sawScope.reset()
		r := httptest.NewRequest(http.MethodGet, "/accounts/acct-1/entries", nil)
		r.Header.Set("Authorization", "Bearer "+operatorKey)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		_, scoped := sawScope.scope.Resolve()
		if scoped {
			t.Fatalf("expected the operator-key caller to be marked Unscoped, got scoped=true")
		}
	})

	t.Run("a valid tenant key is accepted and scoped to that tenant", func(t *testing.T) {
		sawScope.reset()
		r := httptest.NewRequest(http.MethodGet, "/accounts/acct-1/entries", nil)
		r.Header.Set("Authorization", "Bearer known-tenant-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		gotTenantID, scoped := sawScope.scope.Resolve()
		if !scoped || gotTenantID != knownTenantID {
			t.Fatalf("expected scoped tenant_id %q, got scoped=%v tenant_id=%q", knownTenantID, scoped, gotTenantID)
		}
	})

	t.Run("neither the operator key nor a known tenant key refuses 401 unidentified_caller", func(t *testing.T) {
		sawScope.reset()
		r := httptest.NewRequest(http.MethodGet, "/accounts/acct-1/entries", nil)
		r.Header.Set("Authorization", "Bearer never-issued-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		assertUnidentifiedCallerRefusal(t, w)
		if sawScope.called {
			t.Fatalf("expected next handler not to run on refusal")
		}
	})

	t.Run("missing bearer token refuses 401 unidentified_caller", func(t *testing.T) {
		sawScope.reset()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/accounts/acct-1/entries", nil))
		assertUnidentifiedCallerRefusal(t, w)
		if sawScope.called {
			t.Fatalf("expected next handler not to run on refusal")
		}
	})
}

// --- test support --------------------------------------------------------

// errBoom is a sentinel infrastructure error a fake resolver can return, to
// exercise the "resolver failed" refusal path distinctly from "resolver
// genuinely found nothing".
var errBoom = errRecord("boom")

type errRecord string

func (e errRecord) Error() string { return string(e) }

// tenantScopeProbe is a downstream handler double that records whether it
// ran and, if so, the ports.TenantScope it read back off the request
// context -- the observable this step's criteria requires ("verify via a
// downstream test handler that reads the context key").
type tenantScopeProbe struct {
	called bool
	scope  interface {
		Resolve() (string, bool)
	}
}

func (p *tenantScopeProbe) reset() { p.called = false; p.scope = nil }

func (p *tenantScopeProbe) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.called = true
		scope, ok := TenantScopeFromContext(r.Context())
		if !ok {
			http.Error(w, "no tenant scope in context", http.StatusInternalServerError)
			return
		}
		p.scope = scope
		w.WriteHeader(http.StatusOK)
	}
}

func assertUnidentifiedCallerRefusal(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (body: %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response body: %v (raw: %s)", err, w.Body.String())
	}
	if body["error"] != "unidentified_caller" {
		t.Fatalf("expected error=unidentified_caller, got %v", body)
	}
}
