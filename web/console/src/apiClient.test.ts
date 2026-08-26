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
import { createApiClient, AuthRejectedError, FetchFailedError } from "./apiClient";
import { createKeyStorage } from "./keyStorage";
import { STORAGE_KEY } from "./testing/domainTypes";

function fakeVerdictWireShape(overrides: { verdict?: "YES" | "NO"; drifted?: unknown[] } = {}) {
  // Verbatim GET /console/verdict wire shape (RCA-confirmed against the real
  // backend, see confirmed-mismatch table in fix-console-wire-shape-contract
  // step 01-01): `verdict` is the full sentence the server actually emits,
  // not a bare "YES"/"NO" literal; drifted rows carry snake_case
  // `account_id` and stringified decimal amounts, not camelCase numbers.
  const status = overrides.verdict ?? "YES";
  return {
    verdict: status === "YES" ? "Books balance: YES" : "Books balance: NO",
    imbalance_minor: 0,
    entry_count: 6,
    elapsed_ms: 4,
    drifted: overrides.drifted ?? [],
  };
}

function fakeDriftedRowWireShape() {
  // Verbatim drifted[] row shape from the real backend (confirmed-mismatch
  // table): account_id is snake_case; stored/computed/delta are decimal
  // strings, not numbers.
  return {
    account_id: "acct-42",
    stored: "5.00",
    computed: "4.50",
    delta: "0.50",
  };
}

function fakeEntriesWireEnvelope(rows: unknown[]) {
  // Verbatim GET /accounts/{id}/entries wire shape: the response is wrapped
  // in an {"entries": [...]} envelope, not a bare array (confirmed-mismatch
  // table).
  return { entries: rows };
}

function fakeEntryRowWireShape() {
  // Verbatim entries[] row shape: amount and running_balance are decimal
  // strings on the wire (confirmed-mismatch table).
  return {
    amount: "12.34",
    counterparty: "bob-demo",
    recorded_at: "2026-08-20T10:00:00Z",
    running_balance: "100.00",
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

  it("@walking_skeleton @gap fetchVerdict sends the stored operator key and returns the verdict the API answered with, with no accumulated side effect across repeated calls (C4a)", async () => {
    for (const callCount of [1, 2, 5]) {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => fakeVerdictWireShape(),
      });
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const client = createApiClient({ keyStorage: createKeyStorage() });
      const results = [];
      for (let i = 0; i < callCount; i += 1) {
        results.push(await client.fetchVerdict());
      }

      expect(fetchMock).toHaveBeenCalledTimes(callCount);
      for (const call of fetchMock.mock.calls) {
        expect(call).toEqual(
          expect.arrayContaining([
            "/console/verdict",
            expect.objectContaining({
              headers: expect.objectContaining({ Authorization: "Bearer a-valid-operator-key" }),
            }),
          ])
        );
      }
      for (const result of results) {
        expect(result).toEqual(results[0]);
        expect(result.verdict).toBe("YES");
      }
    }
  });

  it("@gap fetchVerdict translates snake_case drifted[] rows with stringified amounts into the domain DriftRow shape (confirmed-mismatch table)", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => fakeVerdictWireShape({ verdict: "NO", drifted: [fakeDriftedRowWireShape()] }),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const client = createApiClient({ keyStorage: createKeyStorage() });
    const result = await client.fetchVerdict();

    expect(result.verdict).toBe("NO");
    expect(result.drifted).toEqual([
      { accountId: "acct-42", stored: 5.0, computed: 4.5, delta: 0.5 },
    ]);
  });

  it("fetchEntries(accountId) asks for exactly the clicked account's entries, with the same operator key attached, and no write method is exposed on the client (Core Principle 12 read/write port-splitting)", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => fakeEntriesWireEnvelope([]),
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
    expect((client as unknown as Record<string, unknown>).postTransfer).toBeUndefined();
    expect((client as unknown as Record<string, unknown>).createAccount).toBeUndefined();
  });

  it("@gap fetchEntries unwraps the {entries: [...]} envelope and translates stringified amount/running_balance into the domain EntryRow shape (confirmed-mismatch table)", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => fakeEntriesWireEnvelope([fakeEntryRowWireShape()]),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const client = createApiClient({ keyStorage: createKeyStorage() });
    const result = await client.fetchEntries("bob-demo");

    expect(result).toEqual([
      { amount: 12.34, counterparty: "bob-demo", recordedAt: "2026-08-20T10:00:00Z", runningBalance: 100.0 },
    ]);
  });

  it("@error a rejected key clears itself so the operator is not stuck retrying a key that will never work", async () => {
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

  it("@property a request that times out and a request that fails outright read identically to the operator -- both become the same fetch-failed signal", async () => {
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
  }, 20_000); // real 8s AbortController budget (SA-D6, fixed constant) x up to
  // 2 fast-check runs -- extends vitest's default 5s test timeout to match;
  // does not change what the test asserts.

  // --- Gap-closure tests (feature-delta.md Self-Completeness Audit) ---
  // C4a (repeated fetchVerdict calls have no accumulated side effect) is
  // covered above, folded into the walking-skeleton test as a parametrized
  // loop over call counts.

  it("@gap @property a malformed verdict response yields a typed failure, never a silent undefined (C6a)", async () => {
    const malformedVerdictBody = fc.oneof(
      // missing `verdict` entirely
      fc.record({
        imbalance_minor: fc.integer(),
        entry_count: fc.integer(),
        elapsed_ms: fc.integer(),
        drifted: fc.constant([]),
      }),
      // `verdict` present but wrong type
      fc.record({
        verdict: fc.oneof(fc.integer(), fc.boolean(), fc.constant(null)),
        imbalance_minor: fc.integer(),
        entry_count: fc.integer(),
        elapsed_ms: fc.integer(),
        drifted: fc.constant([]),
      }),
      // `verdict` present but not one of the two legal values
      fc.record({
        verdict: fc.constantFrom("MAYBE", "unknown", ""),
        imbalance_minor: fc.integer(),
        entry_count: fc.integer(),
        elapsed_ms: fc.integer(),
        drifted: fc.constant([]),
      }),
      fc.constant(null),
      fc.constant("not-even-an-object")
    );

    await fc.assert(
      fc.asyncProperty(malformedVerdictBody, async (body) => {
        globalThis.fetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 200,
          json: async () => body,
        }) as unknown as typeof fetch;

        const client = createApiClient({ keyStorage: createKeyStorage() });
        const outcome = await client.fetchVerdict().then(
          (value) => ({ settled: "resolved" as const, value }),
          (error) => ({ settled: "rejected" as const, error })
        );

        expect(outcome.settled).toBe("rejected");
        if (outcome.settled === "rejected") {
          expect(outcome.error).toBeInstanceOf(FetchFailedError);
        }
      }),
      { numRuns: 15 }
    );
  });

  it("@gap only auth-rejected or fetch-failed signals ever escape the client -- no third error class leaks (C6c)", async () => {
    const arrangements: Array<() => void> = [
      () => {
        globalThis.fetch = vi.fn().mockResolvedValue({
          ok: false,
          status: 401,
          json: async () => ({ error: "unidentified_caller" }),
        }) as unknown as typeof fetch;
      },
      () => {
        globalThis.fetch = vi.fn().mockRejectedValue(new TypeError("network error")) as unknown as typeof fetch;
      },
      () => {
        globalThis.fetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 200,
          json: async () => ({ not: "a verdict" }),
        }) as unknown as typeof fetch;
      },
    ];

    for (const arrangeFetch of arrangements) {
      window.localStorage.setItem(STORAGE_KEY, "a-valid-operator-key");
      arrangeFetch();

      const client = createApiClient({ keyStorage: createKeyStorage() });
      let caught: unknown;
      try {
        await client.fetchVerdict();
      } catch (error) {
        caught = error;
      }

      expect(caught).toBeDefined();
      expect(caught instanceof AuthRejectedError || caught instanceof FetchFailedError).toBe(true);
    }
  });
});
