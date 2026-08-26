// VerdictFetchError -- a failed verdict fetch shows an explicit error, not a
// blank page, and names the fallback path (US-4 AC). Layer: component.
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { VerdictFetchError } from "./VerdictFetchError";
import { FALLBACK_HEALTH_PATH } from "../testing/domainTypes";

// vitest.config.ts sets globals: false, so @testing-library/react's implicit
// afterEach(cleanup) auto-registration never fires (it only self-registers
// when it detects an ambient global `afterEach`). Without explicit cleanup,
// renders from earlier tests in this file stay mounted and collide with
// later tests asserting on identical text.
afterEach(cleanup);

describe("VerdictFetchError -- the operator is told immediately, and shown where else to look", () => {
  it("a failed verdict fetch shows an explicit error, not a blank page", () => {
    render(<VerdictFetchError fallbackPath={FALLBACK_HEALTH_PATH} />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("the error names the fallback path explicitly, not a generic 'try again later'", () => {
    render(<VerdictFetchError fallbackPath={FALLBACK_HEALTH_PATH} />);
    expect(screen.getByText(/GET \/health\/trial-balance/)).toBeInTheDocument();
    expect(screen.queryByText(/try again later/i)).not.toBeInTheDocument();
  });
});
