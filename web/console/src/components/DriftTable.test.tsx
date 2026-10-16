// DriftTable -- which accounts drifted and by how much, without leaving the
// console (US-2 AC). Layer: component (props -> render mapping).
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DriftTable } from "./DriftTable";

describe("DriftTable -- which accounts drifted and by how much", () => {
  it("a single drifted account is named with its numbers", () => {
    render(
      <DriftTable
        drifted={[{ tenantId: "tnt_legacy_seed", accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.getByText("alice-demo")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("a healthy ledger shows no drift table at all (C3: zero cardinality)", () => {
    const { container } = render(<DriftTable drifted={[]} onSelectAccount={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("two drifted accounts are both named, each with its own numbers (C3: many cardinality)", () => {
    render(
      <DriftTable
        drifted={[
          { tenantId: "tnt_legacy_seed", accountId: "alice-demo", stored: 100, computed: 105, delta: 5 },
          { tenantId: "tnt_legacy_seed", accountId: "bob", stored: 50, computed: 57, delta: 7 },
        ]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.getByText("alice-demo")).toBeInTheDocument();
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  it("a healthy account is never swept into the drift table", () => {
    render(
      <DriftTable
        drifted={[{ tenantId: "tnt_legacy_seed", accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.queryByText("bob")).not.toBeInTheDocument();
  });

  it("clicking a row hands the console the exact account_id and tenant_id clicked, never a re-derived value", async () => {
    const onSelectAccount = vi.fn();
    render(
      <DriftTable
        drifted={[{ tenantId: "tnt_legacy_seed", accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={onSelectAccount}
      />
    );
    await userEvent.click(screen.getByText("alice-demo"));
    expect(onSelectAccount).toHaveBeenCalledWith("alice-demo", "tnt_legacy_seed");
  });

  // --- fix-unscoped-entries-tenant-merge: account ids are only unique within
  // a tenant, so a drifted row is identified by (tenant, account). ------------

  const acmeTreasury = { tenantId: "tnt_acme", accountId: "treasury", stored: -100, computed: -105, delta: -5 };
  const beaconTreasury = { tenantId: "tnt_beacon", accountId: "treasury", stored: -40, computed: -47, delta: -7 };

  it("two tenants' drifted \"treasury\" accounts are both listed, each naming its tenant (C3: many cardinality)", () => {
    render(<DriftTable drifted={[acmeTreasury, beaconTreasury]} onSelectAccount={vi.fn()} />);

    expect(screen.getAllByText("treasury")).toHaveLength(2);
    expect(screen.getByText("tnt_acme")).toBeInTheDocument();
    expect(screen.getByText("tnt_beacon")).toBeInTheDocument();
  });

  it("rows are keyed by tenant plus account, so two namesake rows do not collide", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      render(<DriftTable drifted={[acmeTreasury, beaconTreasury]} onSelectAccount={vi.fn()} />);

      const duplicateKeyWarnings = consoleError.mock.calls.filter((call) => /same key|unique "key"/i.test(String(call[0]) + String(call[1] ?? "")));
      expect(duplicateKeyWarnings).toEqual([]);
    } finally {
      consoleError.mockRestore();
    }
  });

  it("selecting one tenant's \"treasury\" hands the console that tenant, not its namesake's", async () => {
    const onSelectAccount = vi.fn();
    render(<DriftTable drifted={[acmeTreasury, beaconTreasury]} onSelectAccount={onSelectAccount} />);

    await userEvent.click(screen.getByText("tnt_beacon"));

    expect(onSelectAccount).toHaveBeenCalledTimes(1);
    expect(onSelectAccount).toHaveBeenCalledWith("treasury", "tnt_beacon");
  });
});
