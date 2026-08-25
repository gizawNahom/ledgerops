// VerdictBanner -- states "Books balance: YES/NO" as the page's first
// sentence, the "Checking the books..." loading state, and the ${fetched_at}
// freshness label (US-1). Pure-function render over props -- no fetch, no
// storage. RED scaffold created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

import type { VerdictStatus } from "../testing/domainTypes";

export interface VerdictBannerProps {
  loading: boolean;
  verdict: VerdictStatus | null;
  fetchedAt: Date | null;
}

export function VerdictBanner(_props: VerdictBannerProps): JSX.Element {
  throw new Error("Not yet implemented -- RED scaffold");
}
