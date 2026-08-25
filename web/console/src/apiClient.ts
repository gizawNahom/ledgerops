// apiClient -- the sole module permitted to call fetch() (DESIGN SA-D4, Core
// Principle 12). Attaches the operator's stored key to every request, treats
// timeout identically to network failure (SA-D6, 8s constant), and clears a
// rejected key on 401 rather than retrying silently.
//
// Read-only port: only fetchVerdict/fetchEntries are exposed (Core Principle
// 12 read/write port-splitting) -- nothing here can write to the ledger.
//
// Failure taxonomy is intentionally narrow (C6c): every failure that escapes
// this module is exactly one of AuthRejectedError or FetchFailedError. A
// malformed response body is not a third class -- it is folded into
// FetchFailedError so callers never see a silent `undefined` (C6a).
import type { DriftRow, EntryRow, VerdictResponse, VerdictStatus } from "./testing/domainTypes";
import { FETCH_TIMEOUT_MS } from "./testing/domainTypes";
import type { KeyStorage } from "./keyStorage";

export interface ApiClient {
  fetchVerdict(): Promise<VerdictResponse>;
  fetchEntries(accountId: string): Promise<EntryRow[]>;
}

export interface ApiClientDeps {
  keyStorage: KeyStorage;
}

// AuthRejectedError -- the operator key was refused (401 unidentified_caller,
// internal/adapters/http/router.go:60-73). By the time this is thrown, the
// rejected key has already been cleared from storage.
export class AuthRejectedError extends Error {
  constructor() {
    super("auth-rejected: the operator key was refused");
    this.name = "AuthRejectedError";
  }
}

// FetchFailedError -- timeout, network failure, non-2xx response, and a
// malformed response body all read identically to the operator (SA-D6):
// one signal, not a distinguishable class per cause.
export class FetchFailedError extends Error {
  readonly cause?: unknown;

  constructor(cause?: unknown) {
    super("fetch failed");
    this.name = "FetchFailedError";
    this.cause = cause;
  }
}

function isVerdictStatus(value: unknown): value is VerdictStatus {
  return value === "YES" || value === "NO";
}

function isDriftRow(value: unknown): value is DriftRow {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.accountId === "string" &&
    typeof candidate.stored === "number" &&
    typeof candidate.computed === "number" &&
    typeof candidate.delta === "number"
  );
}

// parseVerdictWireShape -- translates the snake_case GET /console/verdict
// wire shape into the domain's VerdictResponse. Any field missing or
// wrong-typed folds into FetchFailedError (C6a) instead of producing a
// partially-populated object.
function parseVerdictWireShape(raw: unknown): VerdictResponse {
  if (typeof raw !== "object" || raw === null) {
    throw new FetchFailedError("malformed verdict response body");
  }
  const body = raw as Record<string, unknown>;
  const drifted = body.drifted;

  if (
    !isVerdictStatus(body.verdict) ||
    typeof body.imbalance_minor !== "number" ||
    typeof body.entry_count !== "number" ||
    typeof body.elapsed_ms !== "number" ||
    !Array.isArray(drifted) ||
    !drifted.every(isDriftRow)
  ) {
    throw new FetchFailedError("malformed verdict response body");
  }

  return {
    verdict: body.verdict,
    imbalanceMinor: body.imbalance_minor,
    entryCount: body.entry_count,
    elapsedMs: body.elapsed_ms,
    drifted,
  };
}

function parseEntryRow(raw: unknown): EntryRow {
  if (typeof raw !== "object" || raw === null) {
    throw new FetchFailedError("malformed entry row");
  }
  const row = raw as Record<string, unknown>;
  if (
    typeof row.amount !== "number" ||
    typeof row.counterparty !== "string" ||
    typeof row.recorded_at !== "string" ||
    typeof row.running_balance !== "number"
  ) {
    throw new FetchFailedError("malformed entry row");
  }
  return {
    amount: row.amount,
    counterparty: row.counterparty,
    recordedAt: row.recorded_at,
    runningBalance: row.running_balance,
  };
}

// parseEntriesWireShape -- entries arrive as a bare array (GET
// /accounts/{id}/entries). An empty array is trivially valid (no rows to
// validate); non-array bodies fold into FetchFailedError.
function parseEntriesWireShape(raw: unknown): EntryRow[] {
  if (!Array.isArray(raw)) {
    throw new FetchFailedError("malformed entries response body");
  }
  return raw.map(parseEntryRow);
}

// performGet -- the single fetch call site. Attaches the operator key,
// enforces the fixed 8s timeout (SA-D6), and normalizes every failure into
// exactly the two signals this module is allowed to throw (C6c). No
// accumulated state survives between calls (C4a): every invocation builds
// its own controller and timer from scratch.
async function performGet(keyStorage: KeyStorage, path: string): Promise<unknown> {
  const operatorKey = keyStorage.get() ?? "";
  const controller = new AbortController();

  const timeoutSignal = new Promise<never>((_, reject) => {
    setTimeout(() => {
      controller.abort();
      reject(new FetchFailedError("request exceeded the fixed timeout budget"));
    }, FETCH_TIMEOUT_MS);
  });

  let response: Response;
  try {
    response = (await Promise.race([
      fetch(path, {
        method: "GET",
        headers: { Authorization: `Bearer ${operatorKey}` },
        signal: controller.signal,
      }),
      timeoutSignal,
    ])) as Response;
  } catch (error) {
    if (error instanceof FetchFailedError) {
      throw error;
    }
    throw new FetchFailedError(error);
  }

  if (response.status === 401) {
    keyStorage.clear();
    throw new AuthRejectedError();
  }

  if (!response.ok) {
    throw new FetchFailedError(`unexpected response status ${response.status}`);
  }

  return response.json();
}

export function createApiClient(deps: ApiClientDeps): ApiClient {
  return {
    async fetchVerdict(): Promise<VerdictResponse> {
      const body = await performGet(deps.keyStorage, "/console/verdict");
      return parseVerdictWireShape(body);
    },
    async fetchEntries(accountId: string): Promise<EntryRow[]> {
      const body = await performGet(deps.keyStorage, `/accounts/${accountId}/entries`);
      return parseEntriesWireShape(body);
    },
  };
}

// Re-exported so tests and the composition root reference one constant, not
// a duplicated literal.
export { FETCH_TIMEOUT_MS };
