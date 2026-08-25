// DriftTable -- one row per drifted account, nothing when healthy (US-2).
// Pure-function render over props. RED scaffold created by DISTILL per
// Mandate 7.
export const __SCAFFOLD__ = true;

import type { DriftRow } from "../testing/domainTypes";

export interface DriftTableProps {
  drifted: DriftRow[];
  onSelectAccount: (accountId: string) => void;
}

export function DriftTable(_props: DriftTableProps): JSX.Element | null {
  throw new Error("Not yet implemented -- RED scaffold");
}
