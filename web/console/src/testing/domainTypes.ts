// Domain types module -- Mandate-12 (SSOT + Zero Duplication via Types +
// Services + DSL). Every domain noun used in this feature's test descriptions
// and component props is typed once here; components and tests import from
// this module rather than re-declaring shapes inline.
//
// Not a scaffold -- these are pure type/shape declarations, nothing to fail.

export type VerdictStatus = "YES" | "NO";

export interface DriftRow {
  accountId: string;
  stored: number;
  computed: number;
  delta: number;
}

// Domain shape for GET /console/verdict's JSON contract -- wire-to-domain
// translation lives in apiClient.ts (parseVerdictWireShape); the real
// backend's wire struct is internal/adapters/http/handlers.go:315-320
// (verdictBody), with the verdict sentence produced at handlers.go:332-336
// (verdictBodyFor) and drifted rows shaped by driftWire at handlers.go:302-307.
export interface VerdictResponse {
  verdict: VerdictStatus;
  imbalanceMinor: number;
  entryCount: number;
  elapsedMs: number;
  drifted: DriftRow[];
}

// Domain shape for GET /accounts/{id}/entries -- US-3 AC forbids
// client-side recomputation, so this shape is rendered, never derived.
// Wire-to-domain translation lives in apiClient.ts (parseEntriesWireShape /
// parseEntryRow); the real backend's wire shape is produced by
// entriesToWire at internal/adapters/http/handlers.go:110-122, wrapped in
// the {"entries": [...]} envelope built at handlers.go:104-106.
export interface EntryRow {
  amount: number;
  counterparty: string;
  recordedAt: string;
  runningBalance: number;
}

export type FetchPhase = "loading" | "loaded" | "error";

// Mirrors environments.yaml's 5 target environments -- used to parametrize
// component tests over the manual-dogfood-equivalent preconditions (DDR-2
// forbids automated browser E2E, but component tests can still exercise the
// same precondition shapes: key stored vs. absent, API reachable vs. not).
export type LedgerEnvironment =
  | "clean"
  | "ci"
  | "with-stored-key"
  | "without-stored-key"
  | "api-unreachable";

export const STORAGE_KEY = "ledgerops_console_api_key" as const;

export const FALLBACK_HEALTH_PATH = "GET /health/trial-balance" as const;

export const FETCH_TIMEOUT_MS = 8_000 as const;
