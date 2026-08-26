// VerdictBanner -- states "Books balance: YES/NO" as the page's first
// sentence, the "Checking the books..." loading state, and the ${fetched_at}
// freshness label (US-1). Pure-function render over props -- no fetch, no
// storage.
import type { VerdictStatus } from "../testing/domainTypes";

export interface VerdictBannerProps {
  loading: boolean;
  verdict: VerdictStatus | null;
  fetchedAt: Date | null;
}

function formatFetchedAt(fetchedAt: Date): string {
  return `as of ${fetchedAt.toLocaleTimeString()}`;
}

export function VerdictBanner({ loading, verdict, fetchedAt }: VerdictBannerProps): JSX.Element {
  if (loading || verdict === null || fetchedAt === null) {
    return <p>Checking the books...</p>;
  }

  return (
    <div>
      <p>Books balance: {verdict}</p>
      <p>{formatFetchedAt(fetchedAt)}</p>
    </div>
  );
}
