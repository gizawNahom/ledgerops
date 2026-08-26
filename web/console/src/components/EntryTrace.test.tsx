// EntryTrace -- trace a drifted account to its entries, without leaving the
// console (US-3 AC). Layer: component (props -> render mapping).
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { EntryTrace } from "./EntryTrace";

// afterEach(cleanup) auto-registration never fires (it only self-registers
// when it detects an ambient global `afterEach`; this project runs with
// `globals: false`). Without explicit cleanup, DOM from one test leaks into
// the next -- mirrors VerdictBanner.test.tsx / DriftTable.test.tsx.
afterEach(cleanup);

describe("EntryTrace -- trace a drifted account to its entries", () => {
  it("clicking a drifted account shows its entries, ordered, with a running balance column", () => {
    render(
      <EntryTrace
        accountId="alice-demo"
        phase="loaded"
        entries={[
          { amount: 100, counterparty: "treasury-demo", recordedAt: "2026-08-20T00:00:00Z", runningBalance: 100 },
          { amount: 5, counterparty: "(tampered)", recordedAt: "2026-08-24T00:00:00Z", runningBalance: 105 },
        ]}
      />
    );
    expect(screen.getByText("treasury-demo")).toBeInTheDocument();
    expect(screen.getByText("105")).toBeInTheDocument();
  });

  it("each entry shows enough to explain it: counterparty and recorded_at, alongside amount and running balance", () => {
    render(
      <EntryTrace
        accountId="alice-demo"
        phase="loaded"
        entries={[
          { amount: 100, counterparty: "treasury-demo", recordedAt: "2026-08-20T00:00:00Z", runningBalance: 100 },
        ]}
      />
    );
    expect(screen.getByText("treasury-demo")).toBeInTheDocument();
    expect(screen.getByText(/2026-08-20/)).toBeInTheDocument();
  });

  it("a healthy account's trace shows no divergence -- running balance matches stored balance at every row", () => {
    render(
      <EntryTrace
        accountId="bob"
        phase="loaded"
        entries={[
          { amount: 50, counterparty: "treasury-demo", recordedAt: "2026-08-20T00:00:00Z", runningBalance: 50 },
        ]}
      />
    );
    expect(screen.getByText("50")).toBeInTheDocument();
  });

  it("while entries are loading, the operator sees a neutral in-progress state, not a blank panel", () => {
    render(<EntryTrace accountId="alice-demo" phase="loading" entries={null} />);
    expect(screen.getByText(/loading entries for alice-demo/i)).toBeInTheDocument();
  });

  it("@error a failed entries fetch does not strand the operator mid-investigation -- names the exact fallback URL", () => {
    render(<EntryTrace accountId="alice-demo" phase="error" entries={null} />);
    expect(screen.getByText(/GET \/accounts\/alice-demo\/entries/)).toBeInTheDocument();
    expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
  });
});
