# Shared Artifacts Registry — ledger-core-console

Produced during the comprehensive Phase 2 (Journey Design) redo, per corrected
Decision 3 = "Comprehensive". Every `${variable}` referenced in
`journey-console-visual.md` / `journey-console.yaml` is tracked here with a
single documented source.

| Artifact | Source of truth | Consumers | Owner | Integration risk | Validation |
|---|---|---|---|---|---|
| `${verdict}` | `GET /console/verdict` JSON field `verdict` | S2 verdict sentence (US-1), US-4 error-state absence check | `ledger-core` backend (already contract-tested) | LOW — single already-tested source, SPA never recomputes it | Console string matches JSON field exactly (US-1 AC) |
| `${drifted}` | `GET /console/verdict` JSON field `drifted` (array) | S3 drift table (US-2), click-through source for S4 (US-3) | `ledger-core` backend | LOW — same response as `${verdict}`, one fetch | Row count and fields match JSON array exactly (US-2 AC) |
| `${account_id}` | S3 drift listing row (itself sourced from `${drifted}`) | S4 entry-trace fetch (US-3), directly-navigated URL | Console (derived, not independently fetched) | LOW | Clicking a row fetches entries for that exact `account_id` (US-3 AC) |
| `${entries}` / `${running_balance}` | `GET /accounts/{id}/entries` JSON response | S4 entry-trace table (US-3) | `ledger-core` backend (already contract-tested) | LOW — no client-side recomputation permitted (US-3 AC) | Table renders response fields verbatim |
| `${fetched_at}` **(new, this pass)** | **Client-side browser clock**, captured at the moment the `GET /console/verdict` fetch resolves — NOT a server-supplied field | S1/S2 verdict header freshness label (US-1, new scenario) | Console (client-local only) | MEDIUM — must stay clearly labeled as client-observed time; if a future backend change adds a server timestamp, this artifact's source must be re-pointed there, not silently dual-sourced | Operator can see how long ago the current verdict was fetched, addressing the stale-data risk identified in this pass's mental-model interrogation (§ journey-console-visual.md, "Stale-data state") |
| `${fallback_path}` | Hardcoded string `GET /health/trial-balance`, matching the already-existing endpoint | US-4 error state | Console (static string, not fetched) | LOW — endpoint is fixed and already documented in § Driving ports | Error message names the exact path (US-4 AC) |

## Cross-cutting note on `${fetched_at}`

This is the one genuinely new shared artifact discovered by the comprehensive
depth pass (previously undocumented — the lightweight pass did not interrogate
staleness). It must NOT be confused with a backend field: `GET /console/verdict`
has no timestamp field today (confirmed against the field list in
`docs/feature/ledger-core-console/slices/slice-01-console-states-the-verdict.md`:
`verdict`, `imbalance_minor`, `entry_count`, `elapsed_ms`, `drifted`). Sourcing
`${fetched_at}` client-side keeps the "no new backend work" System Constraint
intact.
