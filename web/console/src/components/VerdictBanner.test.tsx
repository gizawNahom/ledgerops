// VerdictBanner -- the operator's first sentence, before any figures (US-1
// AC). Layer: component (props -> render mapping, jsdom, no fetch).
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { VerdictBanner } from "./VerdictBanner";

// vitest.config.ts sets globals: false, so @testing-library/react's implicit
// afterEach(cleanup) auto-registration never fires (it only self-registers
// when it detects an ambient global `afterEach`). Without explicit cleanup,
// renders from earlier tests in this file stay mounted and collide with
// later tests asserting on identical text ("Books balance: YES").
afterEach(cleanup);

describe("VerdictBanner -- the operator's first sentence, before any figures", () => {
  it("a healthy ledger's console states the verdict in words first", () => {
    render(<VerdictBanner loading={false} verdict="YES" fetchedAt={new Date("2026-08-25T09:14:02Z")} />);
    expect(screen.getByText("Books balance: YES")).toBeInTheDocument();
  });

  it("@error a corrupted ledger's console states the verdict in words first", () => {
    render(<VerdictBanner loading={false} verdict="NO" fetchedAt={new Date("2026-08-25T09:14:02Z")} />);
    expect(screen.getByText("Books balance: NO")).toBeInTheDocument();
  });

  it("an empty ledger still gives a clean answer (C1: boundary/empty domain)", () => {
    render(<VerdictBanner loading={false} verdict="YES" fetchedAt={new Date()} />);
    expect(screen.getByText("Books balance: YES")).toBeInTheDocument();
  });

  it("the operator sees a checking state while loading, never a blank page", () => {
    render(<VerdictBanner loading={true} verdict={null} fetchedAt={null} />);
    expect(screen.getByText(/checking the books/i)).toBeInTheDocument();
    expect(screen.queryByText(/books balance/i)).not.toBeInTheDocument();
  });

  it("once the verdict appears, it is labeled with how long ago it was fetched", () => {
    render(<VerdictBanner loading={false} verdict="YES" fetchedAt={new Date("2026-08-25T09:14:02Z")} />);
    expect(screen.getByText(/ago|as of/i)).toBeInTheDocument();
  });
});
