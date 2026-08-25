# ADR-010 — Console operator API key is pasted and stored client-side

**Status**: Accepted · 2026-08-25 · Feature: `ledger-core-console` · Decision: DESIGN Pre-requisite #2

## Context

`ledger-core-console` (`web/console/`) is a new React SPA that renders the
already-built, already-contract-tested verdict/drift/entry-trace endpoints of
the `ledger-core` backend. Every endpoint it consumes requires the same
seeded operator API key every other surface uses
(`internal/adapters/http/router.go:60-73`, `requireOperatorKey`): a mismatched
or missing key is refused with `401 {"error":"unidentified_caller"}`.

Two hard constraints bound the decision:

- **D10** (`docs/feature/ledger-core-console/feature-delta.md` § Out-of-scope):
  no login UI, no session management, no multi-operator support this pass.
  A companion feature, `operator-authentication`, is the place a real login
  flow belongs.
- **DDR-2 / this feature's Definition of Done item 7**: zero backend code
  changes are permitted. Whatever mechanism is chosen must not require
  touching `internal/adapters/http/`.

The open question DISCUSS deferred to DESIGN (§ Pre-requisites): how does the
operator's API key reach the browser under this static-key model? The user
answered directly, in Guide mode, before this ADR was written: **the operator
pastes/stores it client-side**.

## Decision

A first-load `ApiKeyPrompt` component collects the key via a single
`type="password"` input. On submit, the key is written to
`localStorage['ledgerops_console_api_key']` by a single dedicated module
(`keyStorage`) — no other component touches `localStorage` directly. Every
subsequent request goes through a single `apiClient` module that reads the
stored key and attaches `Authorization: Bearer <key>` to the request,
matching the existing middleware's contract exactly. A `401
unidentified_caller` response clears the stored key and re-renders
`ApiKeyPrompt` with an inline "key rejected" message, so a bad key cannot get
stuck in `localStorage` producing a silent, unexplained failure loop.

No build-time secret baking. No server-side change. The key never leaves the
browser except in the `Authorization` header of a request to the API it
authenticates.

Full component/flow design: `docs/product/architecture/brief.md` §
Application Architecture → Console SPA → "Key delivery flow".

## Alternatives considered

**Env-baked at build time** (`import.meta.env.VITE_OPERATOR_KEY`, compiled
into the static JS bundle). Rejected:

- The key becomes readable by anyone who can view the served bundle's
  source or network tab — a real exposure vector even though today's sole
  consumer is the trusted dogfooding operator themselves; the architecture
  should not normalize baking secrets into client bundles as the only
  implementation on record, since that pattern outlives the single-operator
  context that currently makes it low-stakes.
- Rotating the key requires a full rebuild and redeploy of the static
  bundle. For a key that may need to change (compromise, rotation policy),
  that is a heavyweight recovery path for something `keyStorage.clear()` +
  re-paste solves in one browser action under the chosen design.

**Dev-proxy-injected header** (the proxy or a reverse-proxy layer attaches
`Authorization` server-side; the browser's JS never handles the key at all).
Correctly the most secure of the three — the key never touches client-side
JavaScript, closing the XSS-exposure surface the chosen option accepts.
Rejected anyway:

- It requires an always-on injecting layer in front of *every* environment
  the console is served from, not just `vite dev`. `docs/product/architecture/brief.md`
  § Deployment shape is explicit: **there is no hosted environment** —
  `clean` (developer machine) and `ci` are the entire matrix. Production, to
  the extent the term applies, means the built static bundle served by the
  same single Go binary that exposes the API — there is no reverse-proxy
  tier in that shape to inject a header from.
- Building that tier now, for a single dogfooding operator who already
  holds the key, is premature machinery: it solves a threat model
  (protecting the key from the person using it) that does not exist yet at
  this project's current scale and deployment shape.
- Revisit when either becomes true: a hosted environment appears (giving a
  natural place for an injecting layer to live), or the `operator-authentication`
  companion feature (D10) introduces real multi-user login, at which point
  the trust model this ADR assumes — the sole browser user *is* the
  authorized operator — no longer holds.

## Consequences

- **Positive**: zero backend change (DDR-2 compliant); no build-time secret;
  key rotation is a one-action UX flow (401 → clear → re-prompt), not a
  rebuild; the key is scoped to a single, clearly-named `localStorage` key
  behind one module (`keyStorage`), not scattered ambient access.
- **Negative**: a stored key is exposed to any script that can execute in
  the page's origin (XSS). Accepted because this feature renders no
  user-generated content — the XSS surface is the SPA's own code and its
  two backend dependencies, both already trusted — and because the
  key-holder and the browser-user are the same person under D10's
  single-operator model. This trade-off must be re-examined, not
  silently inherited, if `operator-authentication` introduces a second
  operator who should not hold the first operator's key.
- **Negative**: no server-side revocation signal reaches the console
  automatically — a revoked key is only discovered on the next request's
  401. Acceptable at current request volume (single-digit requests/minute,
  per § System-Level Scope Confirmation); would need reconsideration at
  higher-stakes or higher-latency-to-discovery scenarios.
