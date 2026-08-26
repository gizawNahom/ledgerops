// apiClient.pact.test.ts -- consumer-driven Pact contract tests pinning
// apiClient's wire-shape expectations against the real ledger-core backend.
//
// Hand-written strictly from the RCA-confirmed-mismatch table
// (fix-console-wire-shape-contract, step 01-01) -- NOT captured from a live
// server:
//   - verdict is the full sentence "Books balance: YES"/"Books balance: NO"
//   - drifted[].account_id is snake_case; stored/computed/delta are strings
//   - entries responses are wrapped in an {"entries": [...]} envelope, with
//     entries[].amount / entries[].running_balance as strings
//
// Each interaction below stands up Pact's own local mock provider, runs the
// real apiClient method against it, and on success writes the pact contract
// to web/console/pacts/console-ledgercore.json (Pact's default `dir`
// option, pointed at this path via `dir` below).
//
// apiClient is not yet fixed to parse these real shapes (that is step
// 01-02's GREEN) -- so the client call inside each interaction may reject.
// That rejection is irrelevant to the Pact contract itself: Pact only cares
// whether the REQUEST apiClient made matched what was set up, not whether
// apiClient could parse the RESPONSE. Bypass: rejections are swallowed here
// on purpose (`.catch(() => undefined)`) so a failing parse never masks
// (or is confused with) a Pact mismatch.
import { describe, it, beforeEach, afterEach } from "vitest";
import { PactV3, MatchersV3 } from "@pact-foundation/pact";
import { createApiClient } from "./apiClient";
import { createKeyStorage } from "./keyStorage";
import { STORAGE_KEY } from "./testing/domainTypes";

const { like, eachLike } = MatchersV3;

// Resolved relative to the vitest process cwd (web/console, per package.json
// "test" script) -- Pact's default `dir` option, pointed at
// web/console/pacts/console-ledgercore.json. Avoids a node: import so this
// file needs no @types/node / tsconfig change beyond what's already listed
// in this step's files_to_modify.
const provider = new PactV3({
  consumer: "console",
  provider: "ledgercore",
  dir: "./pacts",
});

// routeToMockServer -- apiClient always issues fetch() against a relative
// path (DESIGN SA-D4: apiClient is the sole fetch() caller, and it never
// knows about an absolute host). Pact's mock server listens on its own
// dynamic port, so this wrapper prefixes each outgoing request with the
// mock server's base URL without touching apiClient itself.
function routeToMockServer(baseUrl: string, realFetch: typeof fetch) {
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const relativePath = typeof input === "string" ? input : input.toString();
    // `signal` is dropped rather than forwarded: apiClient builds it from
    // jsdom's AbortController (test environment), while Node's real fetch
    // (undici) expects its own native AbortSignal class and rejects a
    // cross-realm instance. The pact contract only cares that the request
    // matches -- request-level abort semantics are exercised separately by
    // apiClient.test.ts's own timeout/network-failure property test.
    const initWithoutSignal: RequestInit = { ...init };
    delete initWithoutSignal.signal;
    return realFetch(`${baseUrl}${relativePath}`, initWithoutSignal);
  }) as typeof fetch;
}

describe("apiClient -- consumer-driven Pact contract with the ledger-core backend", () => {
  let originalFetch: typeof fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
    window.localStorage.setItem(STORAGE_KEY, "a-valid-operator-key");
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    window.localStorage.clear();
  });

  it("GET /console/verdict -- returns the verdict as a full sentence with snake_case, stringified drifted rows", async () => {
    provider
      .given("the books are out of balance with one drifted account")
      .uponReceiving("a request for the balance verdict")
      .withRequest({
        method: "GET",
        path: "/console/verdict",
        headers: { Authorization: like("Bearer a-valid-operator-key") },
      })
      .willRespondWith({
        status: 200,
        headers: { "Content-Type": "application/json" },
        body: {
          verdict: "Books balance: NO",
          imbalance_minor: like(50),
          entry_count: like(6),
          elapsed_ms: like(4),
          drifted: eachLike({
            account_id: "acct-42",
            stored: "5.00",
            computed: "4.50",
            delta: "0.50",
          }),
        },
      });

    await provider.executeTest(async (mockServer) => {
      routeToMockServer(mockServer.url, originalFetch);
      const client = createApiClient({ keyStorage: createKeyStorage() });
      // apiClient.ts is not yet fixed for this wire shape (step 01-02) --
      // only the request-matching outcome matters for the pact contract.
      await client.fetchVerdict().catch(() => undefined);
    });
  });

  it("GET /accounts/{id}/entries -- returns entries wrapped in an envelope with stringified amounts", async () => {
    provider
      .given("account alice-demo has at least one entry")
      .uponReceiving("a request for alice-demo's entries")
      .withRequest({
        method: "GET",
        path: "/accounts/alice-demo/entries",
        headers: { Authorization: like("Bearer a-valid-operator-key") },
      })
      .willRespondWith({
        status: 200,
        headers: { "Content-Type": "application/json" },
        body: {
          entries: eachLike({
            amount: "12.34",
            counterparty: "bob-demo",
            recorded_at: "2026-08-20T10:00:00Z",
            running_balance: "100.00",
          }),
        },
      });

    await provider.executeTest(async (mockServer) => {
      routeToMockServer(mockServer.url, originalFetch);
      const client = createApiClient({ keyStorage: createKeyStorage() });
      // Same rationale as above -- apiClient.ts doesn't unwrap the envelope
      // yet; only the request-matching outcome matters here.
      await client.fetchEntries("alice-demo").catch(() => undefined);
    });
  });
});
