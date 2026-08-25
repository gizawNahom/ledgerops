// EntryTrace -- the entries behind a drifted account, ordered, with a
// running balance, and the divergence visible at the offending row (US-3).
// Three states mirroring VerdictBanner's S1/S1a pattern: loading, loaded,
// error. RED scaffold created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

import type { EntryRow, FetchPhase } from "../testing/domainTypes";

export interface EntryTraceProps {
  accountId: string;
  phase: FetchPhase;
  entries: EntryRow[] | null;
}

export function EntryTrace(_props: EntryTraceProps): JSX.Element {
  throw new Error("Not yet implemented -- RED scaffold");
}
