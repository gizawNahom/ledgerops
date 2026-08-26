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
    return (
      <div className="flex items-center gap-3.5 rounded-xl border border-line bg-surface-sunken px-5 py-5">
        <span className="inline-flex items-center gap-1.5 rounded-full border border-line-strong bg-white px-2.5 py-1 font-mono text-xs font-semibold text-ink-muted">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-accent motion-reduce:animate-none" />
          checking
        </span>
        <p className="m-0 text-base font-semibold text-ink">Checking the books...</p>
      </div>
    );
  }

  const isBalanced = verdict === "YES";

  return (
    <div
      className={
        "flex items-center gap-3.5 rounded-xl border px-5 py-5 " +
        (isBalanced ? "border-good-border bg-good-soft" : "border-bad-border bg-bad-soft")
      }
    >
      <span
        className={
          "inline-flex items-center gap-1.5 rounded-full border bg-white px-2.5 py-1 font-mono text-xs font-semibold " +
          (isBalanced ? "border-good-border text-good" : "border-bad-border text-bad")
        }
      >
        <span className={"h-1.5 w-1.5 rounded-full " + (isBalanced ? "bg-good" : "bg-bad")} />
        {verdict.toLowerCase()}
      </span>
      <div>
        <p className="m-0 text-base font-semibold text-ink">Books balance: {verdict}</p>
        <p className="m-0 mt-0.5 font-mono text-xs text-ink-faint">{formatFetchedAt(fetchedAt)}</p>
      </div>
    </div>
  );
}
