// apiClient -- the operator's browser asks the API whether the books balance,
// and every request carries the same operator identity the HTTP API already
// requires (requireOperatorKey, internal/adapters/http/router.go:60-73).
//
// Layer: unit (fetch mocked -- no real network; DDR-2 forbids a real-browser
// probe, and the real-I/O contract for this JSON shape is already asserted at
// tests/acceptance/ledgercore/milestone-04-proof-of-balance.feature over a
// real Postgres-backed server. This file proves apiClient's own wiring:
// header attachment, timeout-as-network-failure equivalence, 401 recovery).
//
// Mandate 9: layer 1 -> PBT full for the timeout/network-failure equivalence.
// Mandate 8: fetchVerdict/fetchEntries are read-only (no local state mutated
// on the happy path) -- state-delta applies to the 401 case, which mutates
// keyStorage; asserted directly against keyStorage's own universe there.
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import fc from "fast-check";
import { createApiClient } from "./apiClient";
import { createKeyStorage } from "./keyStorage";
import { STORAGE_KEY } from "./testing/domainTypes";

function fakeVerdictWireShape() {
  // Verbatim GET /console/verdict wire shape (snake_case on the wire; the
  // client's job is exactly this translation into VerdictResponse).
  return {
    verdict: "YES",
    imbalance_minor: 0,
    entry_count: 6,
    elapsed_ms: 4,
    drifted: [],
  };
}

describe("apiClient -- the operator's browser asks the API whether the books balance", () => {
  let originalFetch: typeof fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
    window.localStorage.setItem(STORAGE_KEY, "a-valid-operator-key");
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it("@walking_skeleton fetchVerdict sends the stored operator key and returns the verdict the API answered with", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => fakeVerdictWireShape(),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const client = createApiClient({ keyStorage: createKeyStorage() });
    const result = await client.fetchVerdict();

    expect(fetchMock).toHaveBeenCalledWith(
      "/console/verdict",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer a-valid-operator-key" }),
      })
    );
    expect(result.verdict).toBe("YES");
  });

  it.skip("fetchEntries(accountId) asks for exactly the clicked account's entries, with the same operator key attached", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [],
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const client = createApiClient({ keyStorage: createKeyStorage() });
    await client.fetchEntries("alice-demo");

    expect(fetchMock).toHaveBeenCalledWith(
      "/accounts/alice-demo/entries",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer a-valid-operator-key" }),
      })
    );
  });

  it.skip("@error a rejected key clears itself so the operator is not stuck retrying a key that will never work", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ error: "unidentified_caller" }),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const keyStorage = createKeyStorage();
    const client = createApiClient({ keyStorage });

    await expect(client.fetchVerdict()).rejects.toThrow();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it.skip("@property a request that times out and a request that fails outright read identically to the operator -- both become the same fetch-failed signal", async () => {
    await fc.assert(
      fc.asyncProperty(fc.constantFrom("timeout", "network-error"), async (failureMode) => {
        globalThis.fetch =
          failureMode === "timeout"
            ? (vi.fn(
                () => new Promise(() => {}) // never resolves -- exercised under AbortController's 8s budget
              ) as unknown as typeof fetch)
            : (vi.fn().mockRejectedValue(new TypeError("network error")) as unknown as typeof fetch);

        const client = createApiClient({ keyStorage: createKeyStorage() });
        await expect(client.fetchVerdict()).rejects.toThrow(/fetch failed/i);
      }),
      { numRuns: 2 }
    );
  });

  it.skip("no write method is exposed on the client -- the console can only ever ask, never change, the books (Core Principle 12 read/write port-splitting)", () => {
    const client = createApiClient({ keyStorage: createKeyStorage() });
    expect((client as unknown as Record<string, unknown>).postTransfer).toBeUndefined();
    expect((client as unknown as Record<string, unknown>).createAccount).toBeUndefined();
  });
});
