// ConsoleApp -- root orchestrator. Holds page-level state (authenticated? /
// loading / verdict / error), decides which component to render, sequences
// the key-gate before any data fetch (US-1..US-4 shell, DESIGN SA-D4). RED
// scaffold created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

import type { KeyStorage } from "../keyStorage";
import type { ApiClient } from "../apiClient";

export interface ConsoleAppProps {
  keyStorage: KeyStorage;
  apiClient: ApiClient;
}

export function ConsoleApp(_props: ConsoleAppProps): JSX.Element {
  throw new Error("Not yet implemented -- RED scaffold");
}
