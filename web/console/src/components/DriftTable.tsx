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
    <tr onClick={() => onSelectAccount(row.accountId)}>
      <td>{row.accountId}</td>
      <td>{row.stored}</td>
      <td>{row.computed}</td>
      <td>{row.delta}</td>
    </tr>
  );
}

export function DriftTable({ drifted, onSelectAccount }: DriftTableProps): JSX.Element | null {
  if (drifted.length === 0) {
    return null;
  }

  return (
    <table>
      <tbody>
        {drifted.map((row) => (
          <DriftTableRow key={row.accountId} row={row} onSelectAccount={onSelectAccount} />
        ))}
      </tbody>
    </table>
  );
}
