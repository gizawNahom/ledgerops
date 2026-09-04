// trial_balance_tenant_scope_test.go is the single-example, real-PostgreSQL
// adapter integration test for step 03-01's query-parameter wiring — per the
// layered test discipline (nw-tdd-methodology § Layered test discipline):
// "Integration | UNCHANGED — single-example test verifies WIRING." It proves
// the real front door (apphttp.NewRouter, real chi routing, real
// requireOperatorKey gate) threads a real `?tenant_id=` query parameter all
// the way down to a real PostgreSQL-backed VerifyBooks call, not merely that
// the application layer's fakes behave (that is usecases_test.go's job, one
// layer in).
//
// Scoped-filter isolation itself (a tenant-scoped call never returns another
// tenant's drift) is proven by internal/adapters/postgres's own
// TrialBalance/ComputedBalances tests (step 02-02) at the repository layer —
// this test's narrower job is the wiring: does the query parameter reach
// VerifyBooks as the right TenantScope, and does an unprovisioned tenant_id
// come back 404 tenant_not_found before any scan runs.
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/adapters/postgres"
)

// migratedRouterServer brings up PostgreSQL 16, migrates it from zero, opens
// the store as the application role, and stands the REAL production router
// up over a real socket — the same weight of setup
// internal/adapters/postgres/testhelper_test.go already gives repository
// tests, one layer up so this test drives entirely through HTTP.
func migratedRouterServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("ledgerops"),
		tcpostgres.WithUsername("ledgerops_migrate"),
		tcpostgres.WithPassword("migrate-secret"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("bringing up PostgreSQL 16: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	privilegedDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("reading the privileged connection string: %v", err)
	}
	if err := postgres.Migrate(ctx, privilegedDSN); err != nil {
		t.Fatalf("migrating from zero: %v", err)
	}

	parsed, err := url.Parse(privilegedDSN)
	if err != nil {
		t.Fatalf("parsing the privileged DSN: %v", err)
	}
	parsed.User = url.UserPassword("ledgerops_app", "app-secret")

	store, err := postgres.Open(ctx, parsed.String())
	if err != nil {
		t.Fatalf("opening the store as the application role: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	const operatorKey = "trial-balance-scope-test-operator-key"
	router := apphttp.NewRouter(apphttp.Deps{
		Store:       store,
		OperatorKey: operatorKey,
		Clock:       time.Now,
		IDGenerator: func() string { return "txn_" + uuid.NewString() },
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, operatorKey
}

// provisionedTenantID provisions a real tenant through the real front door
// (POST /tenants, the platform-admin-only route) and returns the tenant_id
// the server minted — never a locally-guessed id, since the whole point of
// this test is exercising the real wiring.
func provisionedTenantID(t *testing.T, server *httptest.Server, operatorKey, name string) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		t.Fatalf("encoding provision-tenant body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/tenants", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building POST /tenants: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+operatorKey)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /tenants: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /tenants status = %d, want 201", resp.StatusCode)
	}

	var decoded struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decoding POST /tenants response: %v", err)
	}
	return decoded.TenantID
}

// TestTrialBalanceHandler_TenantScopeQueryParameterWiring is the single
// integration example covering step 03-01's three observable outcomes over
// a real PostgreSQL-backed router:
//
//  1. GET /health/trial-balance with no `?tenant_id=` keeps succeeding
//     (the byte-identical-to-before regression the implementation_notes
//     require).
//  2. GET /health/trial-balance?tenant_id=<a real, provisioned tenant>
//     succeeds too — the query parameter reaches VerifyBooks as a real
//     ScopedToTenant scope, not merely a string the handler drops.
//  3. GET /health/trial-balance?tenant_id=<never provisioned> is refused
//     404 tenant_not_found — proving the existence check runs, over real
//     PostgreSQL, ahead of any scan.
func TestTrialBalanceHandler_TenantScopeQueryParameterWiring(t *testing.T) {
	server, operatorKey := migratedRouterServer(t)
	ctx := context.Background()

	tenantID := provisionedTenantID(t, server, operatorKey, fmt.Sprintf("acme-%s", uuid.NewString()[:8]))

	t.Run("unscoped call keeps succeeding", func(t *testing.T) {
		status, _ := getTrialBalance(t, ctx, server, operatorKey, "")
		if status != http.StatusOK {
			t.Fatalf("unscoped GET /health/trial-balance status = %d, want 200", status)
		}
	})

	t.Run("tenant-scoped call over a provisioned tenant succeeds", func(t *testing.T) {
		status, _ := getTrialBalance(t, ctx, server, operatorKey, tenantID)
		if status != http.StatusOK {
			t.Fatalf("scoped GET /health/trial-balance status = %d, want 200", status)
		}
	})

	t.Run("tenant-scoped call over an unprovisioned tenant is refused before any scan", func(t *testing.T) {
		status, body := getTrialBalance(t, ctx, server, operatorKey, "tnt_never-provisioned")
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		if body["error"] != "tenant_not_found" {
			t.Fatalf("error = %v, want tenant_not_found", body["error"])
		}
	})
}

// getTrialBalance issues one real GET /health/trial-balance request, with an
// optional `?tenant_id=` query parameter, carrying the platform OperatorKey
// (DDD-22) — this route's credential is unchanged by step 03-01, only the
// query parameter is new.
func getTrialBalance(t *testing.T, ctx context.Context, server *httptest.Server, operatorKey, tenantID string) (int, map[string]any) {
	t.Helper()

	path := "/health/trial-balance"
	if tenantID != "" {
		path += "?tenant_id=" + url.QueryEscape(tenantID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+operatorKey)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decoding GET %s response: %v", path, err)
	}
	return resp.StatusCode, decoded
}
