# RED classification — ledger-core-console

Pre-DELIVER fail-for-the-right-reason gate output. Ran `npx vitest run`
against `web/console/` on 2026-08-25, after `npm install` and a clean
`tsc --noEmit` pass (zero type errors across all scaffolds and test files).

## Result

```
Test Files  1 failed | 7 skipped (8)
     Tests  1 failed | 30 skipped (31)
```

| File | Enabled tests | Result | Classification |
|---|---|---|---|
| `src/apiClient.test.ts` | 1 (`@walking_skeleton`) | FAIL — `Error: Not yet implemented -- RED scaffold` at `src/apiClient.ts:24` | **MISSING_FUNCTIONALITY (RED)** — assertion-shaped `Error` thrown from the scaffold body, not an import/collection error |
| `src/apiClient.test.ts` (4 remaining) | 0 | skipped | one-at-a-time strategy, per Mandate 7 lifecycle |
| `src/keyStorage.test.ts` | 0 | skipped | same |
| `src/components/*.test.tsx` (6 files) | 0 | skipped | same |

## Classification detail

The one enabled scenario (`@walking_skeleton fetchVerdict sends the stored
operator key and returns the verdict the API answered with`) fails because
`createApiClient(...).fetchVerdict()` throws `Error("Not yet implemented --
RED scaffold")` — the exact RED-scaffold shape specified by Mandate 7's
TypeScript convention (`throw new Error(...)`, not `NotImplementedError` or
an import failure). No test file raised `ReferenceError` / module-resolution
error / collection error during `vitest run`'s collect phase — all 8 files
collected and skipped/ran cleanly. `tsc --noEmit` reports zero errors,
confirming the scaffolds and test files are type-correct, not merely
"parses."

**Verdict: genuine RED, not BROKEN.** Zero scenarios fall into
`IMPORT_ERROR` / `FIXTURE_BROKEN` / `SETUP_FAILURE` / `WRONG_ASSERTION` /
`OBSERVABLE_NOT_AT_PORT`. Handoff to DELIVER is not blocked by this gate.

## How to reproduce

```
cd web/console
npm install
npx tsc --noEmit   # zero errors
npx vitest run     # 1 failed (RED, expected) | 30 skipped (30) | 8 files
```
