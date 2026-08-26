// EntryTrace -- the entries behind a drifted account, ordered, with a
// running balance, and the divergence visible at the offending row (US-3).
// Three states mirroring VerdictBanner's loading/loaded/error pattern.
// Pure-function render over fetched state -- fetch itself is delegated to
// apiClient by the caller, never inlined here.
import type { EntryRow, FetchPhase } from "../testing/domainTypes";

export interface EntryTraceProps {
  accountId: string;
  phase: FetchPhase;
  entries: EntryRow[] | null;
}

function entriesFetchUrl(accountId: string): string {
  return `GET /accounts/${accountId}/entries`;
}

function formatAmount(amount: number): string {
  return amount >= 0 ? `+${amount}` : `${amount}`;
}

function EntryTraceRow({ entry }: { entry: EntryRow }): JSX.Element {
  return (
    <tr className="border-b border-line last:border-b-0">
      <td
        className={
          "whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums " +
          (entry.amount >= 0 ? "text-good" : "text-bad")
        }
      >
        {formatAmount(entry.amount)}
      </td>
      <td className="whitespace-nowrap px-4 py-3 font-mono text-ink">{entry.counterparty}</td>
      <td className="whitespace-nowrap px-4 py-3 font-mono text-ink-muted">{entry.recordedAt}</td>
      <td className="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums text-ink">
        {entry.runningBalance}
      </td>
    </tr>
  );
}

export function EntryTrace({ accountId, phase, entries }: EntryTraceProps): JSX.Element {
  if (phase === "loading") {
    return (
      <div className="flex items-center gap-2.5 rounded-lg border border-line bg-surface-sunken px-4 py-6 text-sm text-ink-muted">
        <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-line-strong border-t-accent motion-reduce:animate-none" />
        <p className="m-0">Loading entries for {accountId}...</p>
      </div>
    );
  }

  if (phase === "error" || entries === null) {
    return (
      <p className="m-0 rounded-lg border border-bad-border bg-bad-soft px-4 py-3 text-sm text-bad">
        Could not load entries. Fallback: {entriesFetchUrl(accountId)}
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-line">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-line bg-surface-sunken">
            <th className="whitespace-nowrap px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Amount
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Counterparty
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Recorded At
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Running Balance
            </th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <EntryTraceRow key={`${entry.counterparty}-${entry.recordedAt}`} entry={entry} />
          ))}
        </tbody>
      </table>
    </div>
  );
}
