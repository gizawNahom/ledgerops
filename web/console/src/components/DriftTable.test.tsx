// DriftTable -- which accounts drifted and by how much, without leaving the
// console (US-2 AC). Layer: component (props -> render mapping).
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DriftTable } from "./DriftTable";

describe("DriftTable -- which accounts drifted and by how much", () => {
  it.skip("a single drifted account is named with its numbers", () => {
    render(
      <DriftTable
        drifted={[{ accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.getByText("alice-demo")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it.skip("a healthy ledger shows no drift table at all (C3: zero cardinality)", () => {
    const { container } = render(<DriftTable drifted={[]} onSelectAccount={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it.skip("two drifted accounts are both named, each with its own numbers (C3: many cardinality)", () => {
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

  it.skip("a healthy account is never swept into the drift table", () => {
    render(
      <DriftTable
        drifted={[{ accountId: "alice-demo", stored: 100, computed: 105, delta: 5 }]}
        onSelectAccount={vi.fn()}
      />
    );
    expect(screen.queryByText("bob")).not.toBeInTheDocument();
  });

  it.skip("clicking a row hands the console the exact account_id clicked, never a re-derived value", async () => {
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
