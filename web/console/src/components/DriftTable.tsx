// DriftTable -- one row per drifted account, nothing when healthy (US-2).
// Pure-function render over props.

import type { DriftRow } from "../testing/domainTypes";

export interface DriftTableProps {
  drifted: DriftRow[];
  onSelectAccount: (accountId: string) => void;
}

function DriftTableRow({
  row,
  onSelectAccount,
}: {
  row: DriftRow;
  onSelectAccount: (accountId: string) => void;
}): JSX.Element {
  return (
    <tr
      onClick={() => onSelectAccount(row.accountId)}
      className="cursor-pointer transition-colors last:border-b-0 [&:not(:last-child)]:border-b [&:not(:last-child)]:border-line hover:bg-accent-soft"
    >
      <td className="whitespace-nowrap px-4 py-3 font-mono text-ink">{row.accountId}</td>
      <td className="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums text-ink">
        {row.stored}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums text-ink">
        {row.computed}
      </td>
      <td
        className={
          "whitespace-nowrap px-4 py-3 text-right font-mono tabular-nums " +
          (row.delta < 0 ? "font-semibold text-bad" : "text-ink")
        }
      >
        {row.delta}
      </td>
    </tr>
  );
}

export function DriftTable({ drifted, onSelectAccount }: DriftTableProps): JSX.Element | null {
  if (drifted.length === 0) {
    return null;
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-line">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-line bg-surface-sunken">
            <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Account ID
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Stored
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Computed
            </th>
            <th className="whitespace-nowrap px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-wide text-ink-faint">
              Delta
            </th>
          </tr>
        </thead>
        <tbody>
          {drifted.map((row) => (
            <DriftTableRow key={row.accountId} row={row} onSelectAccount={onSelectAccount} />
          ))}
        </tbody>
      </table>
    </div>
  );
}
