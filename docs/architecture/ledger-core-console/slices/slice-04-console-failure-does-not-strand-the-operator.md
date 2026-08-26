# Slice 04 — Console failure does not strand the operator

**Job**: J4 | **Story**: US-4 | **WS strategy**: no automated coverage possible under DDR-2 — manual dogfood only

## Goal

When the console's own verdict fetch fails, the operator is told immediately
and shown the fallback path to the same answer, per `verify-the-books.yaml`
S1 error path: "the console must not be the only path to the verdict."

## IN scope

- Detect a failed or timed-out `GET /console/verdict` request (network
  error, 5xx, or timeout)
- Render an explicit, bounded-time error state — not an indefinite spinner,
  not a blank page
- Error state text names `GET /health/trial-balance` as a working alternative
- Reloading after the API recovers returns the console to normal US-1
  behavior with no stale error state

## OUT scope

- Automatic retry/polling logic (a manual reload is sufficient for a
  single-operator dogfood tool; no evidence yet that automatic retry earns
  its complexity)
- In-page proxying or embedding of the `/health/trial-balance` response —
  the fallback is named, not fetched and rendered inline, keeping this
  slice's scope to error-state UX only

## Learning hypothesis

Disproves "a console that is a thin JSON client needs no bespoke
fetch-failure design, because the browser's default failure behavior is
good enough" if a plain unhandled fetch rejection (blank page, console
error only visible in devtools) turns out to be what ships without explicit
design attention here — which is precisely the failure mode this slice
exists to prevent. Confirms the hypothesis that explicit error-state design
is required if, absent this slice, the default behavior is silent or
indefinite (spinner never resolves, page stays blank).

Note on scope honesty: this scenario is narrower than the journey's literal
S1 error path ("console unreachable" — the page itself doesn't load, e.g.
the server hosting `web/console/` is down). If the *page* can't load, no
SPA code can render anything — that failure mode has no in-app fix and is
an operational/runbook concern, not a story. This slice instead covers the
case where the page loads but its data fetch fails, which is the part a
story can actually build.

## Acceptance criteria

- [ ] A failed or timed-out `GET /console/verdict` request produces a
      visible error message within a bounded time (define a concrete
      timeout in DESIGN — this brief does not prescribe one)
- [ ] The error message names `GET /health/trial-balance` explicitly
- [ ] No indefinite spinner and no blank page on fetch failure
- [ ] Reloading the console once the API is reachable again shows the
      verdict normally, matching US-1's acceptance criteria
- [ ] `make demo-console-04` stops the API mid-session, confirms the error
      state appears and names the fallback, then restarts the API and
      confirms recovery — run by hand, per DDR-2 (no automated coverage)

## Data

No account data required — this slice is pure fetch-failure UX, exercised by
stopping the backend process rather than manipulating ledger state.

## Dependencies

Slice 01 (the fetch this slice's failure handling wraps).

## Effort

≤1 day. Reference class: one error boundary / catch branch around the
existing fetch call from slice 01, plus a fixed message string.
