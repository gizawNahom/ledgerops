# ledgerops — StoryBrand brandscript (draft 2)

Revised after an independent review against the README, `vision.md` and the
persona. Decisions taken: the integrating engineer is the only hero; "Run it"
is the single call to action; calm, exact tone; wallets and credits lead;
multi-leg stays a "not yet" line, not a selling point. The persona is an
unvalidated archetype, so treat every line as a hypothesis the owner can
change.

Rule for all copy: claim only what the README supports. Say **check** or
**verify**, never **prove**, until independent verification (roadmap item 3)
exists.

## 1. Character

The hero is a backend engineer building something that holds and moves
balances: a wallet, credits or loyalty points. Not a payments specialist, and
doesn't want to become one. Talks to the ledger over HTTP.

**Wants:** transfers that succeed or fail cleanly, and a way to check the
books are right.

## 2. Problem

- **External:** they need balances, and the obvious route is a `balance`
  column plus two `UPDATE` statements.
- **Villain:** silent failure. A partial write, a retry that posts twice, or
  two requests spending the same money. Value is created or destroyed and
  nobody can reconstruct what happened.
- **Internal:** "this works, but would I know if it didn't?"
- **Why it matters:** a system that holds other people's balances should be
  able to show it didn't lose any.

## 3. Guide

- **Empathy:** you don't want to become a ledger expert; you want a transfer
  to be one API call.
- **Authority, as mechanism:**
  - transfers sum to zero, in integer minor units, decided by a pure domain
    core
  - sender and recipient rows are locked in a fixed order, so concurrent
    transfers can't spend the same money
  - `UPDATE` and `DELETE` on entries are revoked from the service's database
    role, with a trigger as a second layer
  - a repeated `Idempotency-Key` returns the original transaction
  - `GET /health/trial-balance` checks the whole ledger on demand and names
    any account that drifts
  - property, race and crash tests run in CI
- **Honesty, up front:** one currency; two-leg transfers only; no holds or
  webhooks yet; not yet run in production; the trial-balance check is run by
  the service itself, and independent verification is on the roadmap. One
  busy account is the first performance limit (p95 923 ms against a 500 ms
  target in the last load test, locks being the cause).

## 4. Plan

1. **Run it.** `make up`, then `make demo-01`. Docker and curl; CI times the
   demo run on every push and reports it (target: under five minutes). Build
   from source until the pull-and-run steps for the release-candidate image
   are confirmed.
2. **Move value.** Post a transfer. Retry it with the same key and nothing
   posts twice. Overdraw it and get a refusal naming the shortfall.
3. **Check the books.** Ask for the trial balance and get a verdict.

## 5. Call to action

- **Primary (the only button):** Run it.
- **Text links:** Read the guarantees · See the checks you can run · Compare with
  other ledgers.

## 6. Stakes

- A double charge after a timeout and retry.
- A negative balance shipped under load.
- Retrofitting a real ledger onto production data that is already wrong.

## 7. Success

- A transfer is one API call that succeeds or fails cleanly.
- Retries are safe when the caller sends an idempotency key.
- "Do the books balance?" has a one-request answer.
- They started with the ledger instead of retrofitting one.

## Copy lines (draft)

- **Hero:** A double-entry ledger for your wallet or credits system.
- **Sub:** No overdrafts, safe retries, append-only history, and a one-call
  check that the books balance. An HTTP service you run yourself.
- **Problem line:** A `balance` column works until it doesn't — and when it
  fails, it fails silently.
- **Plan line:** Run it. Move value. Check the books.
- **Who it's for, one line:** For teams that need balances and want to be
  able to show the numbers add up. If you need very high throughput, holds or
  multi-currency today, see the comparison.

## Left out on purpose

- "Prove" / "proof" as a claim about the trial balance (the README now says
  "check" too).
- A container-image install path, until its steps are confirmed on a clean
  machine (a signed release-candidate image is published).
- Marketplace framing and anything needing more than two legs.
- Auditor and regulator promises; the vision makes none.
