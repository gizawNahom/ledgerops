/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// DISTILL decision (ledger-core-console, 2026-08-25): Vitest is the idiomatic
// test-runner pairing for a Vite 5 + React 18 project (DESIGN SA-D1). Chosen
// over Jest to avoid a second bundler config (Jest would need its own
// transform pipeline duplicating what Vite already does) -- recorded here,
// not silently assumed. See feature-delta.md Wave: DISTILL / [REF] Toolchain.
export default defineConfig({
  plugins: [react()],
  server: {
    // Mirrors docs/product/architecture/brief.md Console SPA -- Dev-time
    // proxy: exactly the three path prefixes this feature consumes, no
    // catch-all, local-dev-tooling-only (DDR-2 / SA-D3).
    proxy: {
      "/console/verdict": "http://localhost:8080",
      "/accounts": "http://localhost:8080",
      "/health": "http://localhost:8080",
    },
  },
  build: {
    outDir: "dist",
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/setupTests.ts"],
    globals: false,
    css: false,
  },
});
