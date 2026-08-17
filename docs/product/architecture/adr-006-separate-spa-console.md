# ADR-006 — Operations console is a separate TypeScript SPA

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: DDD-4

## Context

Slices 04 and 05 need an operator-facing surface: a verdict on whether the books
balance, a list of drifted accounts, and a drill-down into an account's entries.
The backend is Go. The user requires the frontend to be TypeScript.

The journey `verify-the-books.yaml` fixes the information hierarchy — verdict
stated in words first, detail second — which is the part that matters. What
remained open was the delivery mechanism.

## Decision

A separate TypeScript SPA under `web/console/`, communicating with the Go API
over JSON and authenticated with the operator API key.

Framework choice (React, Vue, Svelte) is **deferred to DELIVER**. It is not
architecturally significant at this size: the console is one page with a verdict
and two tables, and no architectural decision depends on which framework renders
it.

## Alternatives considered

**Go serves a built TS bundle** — a single deployable, with Go embedding the
compiled frontend assets. This was the recommended option for operational
simplicity: one process to run, one artifact to deploy, strongest fit for the
"clone and run in five minutes" KPI. Not chosen; the user selected a separate
SPA. Recorded because it remains the easy path back if the two-toolchain setup
proves burdensome, and it does not require rewriting the frontend — only changing
how it is served.

**Server-rendered HTML with TypeScript enhancement** — Go templates render the
verdict and tables. Least scaffolding, fastest route to slice 04, and adequate
for the information hierarchy the journey specifies. Rejected: it puts the least
TypeScript on display, which conflicts with the stated requirement.

## Consequences

- **Slice 04's one-day ceiling is at risk.** A second toolchain, build step, and
  dev server is substantial scaffolding for a page described in its own brief as
  "one page with one verdict." This is the single most likely place the slice
  plan breaks, and it is flagged rather than discovered on the day.
- CORS or a dev proxy is now a real concern in local development.
- The "clone and run in five minutes" KPI now depends on the frontend build
  working on a clean machine — worth an explicit check.
- Type sharing between Go and TypeScript is manual unless a generator is
  introduced. Not proposed now; noted as a future option if the API surface
  grows.
- The console is a genuine API client, which means the API gets exercised by
  something other than tests. That is a real benefit and partly offsets the cost.
