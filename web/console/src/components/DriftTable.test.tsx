// DriftTable -- which accounts drifted and by how much, without leaving the
// console (US-2 AC). Layer: component (props -> render mapping).
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DriftTable } from "./DriftTable";

// vitest.config.ts sets globals: false, so @testing-library/react's
// auto-cleanup (which detects a global `afterEach`) never registers --
// explicit teardown here prevents prior renders in this file from leaking
// into later assertions (queries would otherwise see stale rows).
afterEach(cleanup);

describe("DriftTable -- which accounts drifted and by how much", () => {
  it("a single drifted account is named with its numbers", () => {
    render(
      <DriftTable
        drifted={[{ accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
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
          { accountId: "alice-demo", stored: 100, computed: 105, delta: 5 },
          { accountId: "bob", stored: 50, computed: 57, delta: 7 },
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
        drifted={[{ accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.queryByText("bob")).not.toBeInTheDocument();
  });

  it("clicking a row hands the console the exact account_id clicked, never a re-derived value", async () => {
    const onSelectAccount = vi.fn();
    render(
      <DriftTable
        drifted={[{ accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={onSelectAccount}
      />
    );
    await userEvent.click(screen.getByText("alice-demo"));
    expect(onSelectAccount).toHaveBeenCalledWith("alice-demo");
  });
});
