// apiClient -- the sole module permitted to call fetch() (DESIGN SA-D4, Core
// Principle 12). Attaches the operator's stored key to every request, treats
// timeout identically to network failure (SA-D6, 8s constant), and clears a
// rejected key on 401 rather than retrying silently. RED scaffold created by
// DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

import type { VerdictResponse, EntryRow } from "./testing/domainTypes";
import { FETCH_TIMEOUT_MS } from "./testing/domainTypes";
import type { KeyStorage } from "./keyStorage";

export interface ApiClient {
  fetchVerdict(): Promise<VerdictResponse>;
  fetchEntries(accountId: string): Promise<EntryRow[]>;
}

export interface ApiClientDeps {
  keyStorage: KeyStorage;
}

export function createApiClient(_deps: ApiClientDeps): ApiClient {
  return {
    async fetchVerdict(): Promise<VerdictResponse> {
      throw new Error("Not yet implemented -- RED scaffold");
    },
    async fetchEntries(_accountId: string): Promise<EntryRow[]> {
      throw new Error("Not yet implemented -- RED scaffold");
    },
  };
}

// Re-exported so tests and the composition root reference one constant, not
// a duplicated literal.
export { FETCH_TIMEOUT_MS };
