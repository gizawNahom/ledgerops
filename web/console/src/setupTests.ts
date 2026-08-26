// Vitest + Testing Library setup -- RTL's jest-dom matchers (toBeInTheDocument,
// etc.) registered globally for every component test in this package.
//
// vite.config.ts sets test.globals: false, so RTL's auto-cleanup (which only
// self-registers when it detects an ambient global `afterEach`) never fires
// on its own. Registering the teardown here -- once -- means every component
// test file gets DOM cleanup between tests without duplicating the
// afterEach(cleanup) boilerplate per file.
import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
});
