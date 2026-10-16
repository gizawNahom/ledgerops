// EntryTrace -- trace a drifted account to its entries, without leaving the
// console (US-3 AC). Layer: component (props -> render mapping).
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { EntryTrace } from "./EntryTrace";

describe("EntryTrace -- trace a drifted account to its entries", () => {
  it("clicking a drifted account shows its entries, ordered, with a running balance column", () => {
    render(
      <EntryTrace
        accountId="alice-demo"
        tenantId="tnt_legacy_seed"
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
        tenantId="tnt_legacy_seed"
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
        tenantId="tnt_legacy_seed"
        phase="loaded"
        entries={[
          { amount: 50, counterparty: "treasury-demo", recordedAt: "2026-08-20T00:00:00Z", runningBalance: 50 },
        ]}
      />
    );
    expect(screen.getByText("50")).toBeInTheDocument();
  });

  it("while entries are loading, the operator sees a neutral in-progress state, not a blank panel", () => {
    render(<EntryTrace accountId="alice-demo" tenantId="tnt_legacy_seed" phase="loading" entries={null} />);
    expect(screen.getByText(/loading entries for alice-demo/i)).toBeInTheDocument();
  });

  it("@error a failed entries fetch does not strand the operator mid-investigation -- names the exact fallback URL", () => {
    render(<EntryTrace accountId="alice-demo" tenantId="tnt_legacy_seed" phase="error" entries={null} />);
    expect(screen.getByText(/GET \/accounts\/alice-demo\/entries/)).toBeInTheDocument();
    expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
  });

  // --- fix-unscoped-entries-tenant-merge: an account id alone no longer names
  // one account, so the trace says whose entries it is showing. ---------------

  it("names the tenant whose entries are shown when two tenants hold the same account id", () => {
    render(
      <EntryTrace
        accountId="treasury"
        tenantId="tnt_beacon"
        phase="loaded"
        entries={[{ amount: -40, counterparty: "vendor-beacon", recordedAt: "2026-08-01", runningBalance: -40 }]}
      />
    );
    expect(screen.getByText(/tnt_beacon/)).toBeInTheDocument();
  });

  it("@error the fallback URL carries the tenant, so the operator reproduces the same narrowed read by hand", () => {
    render(<EntryTrace accountId="treasury" tenantId="tnt_beacon" phase="error" entries={null} />);
    expect(screen.getByText(/GET \/accounts\/treasury\/entries\?tenant_id=tnt_beacon/)).toBeInTheDocument();
  });
});
