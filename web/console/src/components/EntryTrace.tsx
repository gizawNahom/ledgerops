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
    <tr>
      <td>{formatAmount(entry.amount)}</td>
      <td>{entry.counterparty}</td>
      <td>{entry.recordedAt}</td>
      <td>{entry.runningBalance}</td>
    </tr>
  );
}

export function EntryTrace({ accountId, phase, entries }: EntryTraceProps): JSX.Element {
  if (phase === "loading") {
    return <p>Loading entries for {accountId}...</p>;
  }

  if (phase === "error" || entries === null) {
    return <p>Could not load entries. Fallback: {entriesFetchUrl(accountId)}</p>;
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Amount</th>
          <th>Counterparty</th>
          <th>Recorded At</th>
          <th>Running Balance</th>
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <EntryTraceRow key={`${entry.counterparty}-${entry.recordedAt}`} entry={entry} />
        ))}
      </tbody>
    </table>
  );
}
