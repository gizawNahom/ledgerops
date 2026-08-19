# ADR-008 — The sealed refusal set spans three decision sites, and status follows the site

**Status**: Accepted · 2026-08-19 · Feature: ledger-core · Decisions: DDD-17, DDD-18, DDD-19
**Extends**: ADR-007 (DDD-12, sealed violation taxonomy)
**Closes**: AT-completeness gaps C2b and C6a (`feature-delta.md` § AT completeness audit)

## Context

DDD-12 seals the set of ways the ledger says no: nothing can produce a refusal
that is not already a member. That property is what makes a missing member a
hard stop rather than an implementer's judgement call — and the completeness
audit found two places where the set is short.

**C2b** — `POST /accounts` naming an account that is already open has no member
and no journey error path.

**C6a** — a malformed body, a non-numeric or over-scale amount, and a currency
mismatch have no member. The gap is sharpened by a live contradiction:
`feature-delta.md` § Driving adapter coverage declares that `POST /transfers`
answers **400**, and the only declared 400 was a missing idempotency key, so a
documented status had no reachable cause for a malformed payload.

Underneath both sits an unstated structural fact. The domain's
`ViolationKind` (`internal/domain/violation.go`) holds four members; the
acceptance suite's `RefusalKind` (`domain_types.go`) holds six. The extra two —
`missing_idempotency_key`, `unidentified_caller` — are decided outside the pure
core and always were. The set was never single. It was sealed at one site and
described as though it were sealed everywhere, which is why questions that land
between sites had nowhere to be answered.

## Decision

**The sealed set is one wire vocabulary with three declared decision sites**
(DDD-17). Every member names exactly one site. Nothing outside the table may
appear in an error body.

| Member | Decided at | Status | Body carries |
|---|---|---|---|
| `malformed_request` | HTTP adapter | **400** | `field`, `detail` |
| `missing_idempotency_key` | HTTP adapter | **400** | — |
| `unidentified_caller` | HTTP adapter (auth middleware) | **401** | — |
| `account_not_found` | Domain core (`Post`) | **404** | `account_id` |
| `account_already_exists` | Domain core (`OpenAccount`) | **409** | `account_id` |
| `idempotency_key_conflict` | Application shell | **409** | — |
| `invalid_amount` | Domain core (`NewMoney` / `Post`) | **422** | `amount` |
| `insufficient_funds` | Domain core (`Post`) | **422** | `available`, `requested` |
| `currency_mismatch` | Domain core (`Post`) | **422** | `from_currency`, `to_currency` |

`unbalanced` stays a member of `domain.ViolationKind` and is deliberately **not**
a wire member. It guards the Transaction smart constructor against a defect in
the rulebook, and no caller input can reach it: the adapter supplies a command,
never an entry list. If it ever escapes it is a bug in `Post`, answered as
**500** — a caller who is told "your entries do not sum to zero" when they
supplied no entries has been handed a refusal they cannot act on.

**One member already disagrees with itself, and this fixes it.** The wire value
for the unknown-account refusal is **`account_not_found`**. `post-a-transfer.yaml`
declares `404 {error: account_not_found}` and `internal/domain/violation.go`
declares `UnknownAccount ViolationKind = "account_not_found"`; the acceptance
suite's mirror (`domain_types.go:93`) declares `unknown_account`. Two artifacts
against one, and the two are the journey the behaviour was promised in and the
scaffold the behaviour is decided in. The Go identifier stays `UnknownAccount` in
both places — only the string on the wire is settled here. The mirror needs a
one-line correction, which belongs to whoever owns the suite; flagged, not
edited.

**Status follows the decision site** (DDD-17). The rule, so that the next member
does not need a debate:

- **400** — the request could not be understood as a command.
- **401** — the caller could not be identified.
- **404** — the command named something that does not exist.
- **409** — the identifier supplied is already bound to something else.
- **422** — the command was understood and the rules refuse it.

**Opening an account that is already open is refused, not replayed** (DDD-18).
New member `account_already_exists`, **409**, naming the account. It is decided
in the pure core: the shell reads the account (or its absence), `OpenAccount`
decides, and a unique constraint on the account name makes the answer hold under
concurrent opens, exactly as the unique key constraint does for I7 (ADR-005).
The two paths converge on the same member, so a race and a repeat are
indistinguishable to the caller, which is the point.

**Malformed payloads split at the purity boundary** (DDD-19):

| Sub-case | Answer | Why |
|---|---|---|
| Invalid JSON, missing required field, unknown field | `malformed_request` · 400 | Not a command. The core is never reached, because nothing can be built to hand it |
| Non-numeric amount (`"abc"`, `null`, wrong JSON type) | `malformed_request` · 400 | A decoding failure, same class as above |
| Over-scale amount (`50.001`) or beyond int64 minor units | `invalid_amount` · 422 | Decodable, but not a legal `Money`. Scale is a property of the currency, and the currency is domain knowledge — an adapter that rejected it would have to hold the ledger's scale rules |
| Currency mismatch between the two accounts | `currency_mismatch` · 422 | See below |

The adapter parses the *lexical* form of an amount — sign, digits, decimal
point — and hands the result to `NewMoney`, which decides legality. This keeps
existing behaviour intact: `0.00` and `-1.00` are already `invalid_amount`/422
and are also "decodable but illegal". Over-scale joins them rather than earning
a member of its own.

**Currency mismatch is a domain refusal** (DDD-19). I1 requires entries to sum
to zero *per currency*. A two-leg transfer between accounts in different
currencies produces `+50.00 EUR` and `-50.00 USD`, which cannot sum to zero per
currency for any amount. It is not disallowed by policy; it is unsatisfiable by
the invariant. `Post` receives both locked account snapshots and therefore both
currencies, so it is the only place that can decide it without a second read.

`currency_mismatch` is **declared and currently unreachable through the driving
ports**, because every account is opened in the ledger's single configured
currency and `POST /accounts` takes no currency field. That unreachability is
the mechanism by which "multi-currency transactions, out of scope" is enforced
rather than merely asserted. Its coverage belongs at layer 1: § PBT obligations
already declares "relax the same-currency assumption" over `domain.Post`, and
this member is what that obligation now asserts against. Adding a currency field
to `POST /accounts` is the smallest change that would make it reachable at layer
3, and is recorded as an open question rather than taken here — it is an API
surface expansion DISCUSS excluded.

## Alternatives considered

**Idempotent success on repeat `POST /accounts`** — return 200 and the existing
account. Attractive on consistency grounds: the feature already treats repeat
submission as first-class (I7, ADR-005), migrations and `docker compose up` are
both required idempotent, and journey step S1 wants account creation to be
"boring and forgettable".

Rejected on the asymmetry the surface similarity hides. ADR-005's whole design
rests on the caller *declaring* a retry by supplying a key; the fingerprint
exists so that a key reused for a different payload fails loudly rather than
silently not doing what was asked. `POST /accounts` carries no key, so the
service cannot tell a retry from a name collision between two independent
callers. Guessing "retry" hands the second caller an account somebody else
created — after which it posts value into a balance it does not own. That is the
same failure ADR-005 rejected, in a costlier place.

**Idempotent when the declared kind matches, 409 when it differs** — the closest
analogue to ADR-005's key-plus-fingerprint, and the strongest rejected option.
Rejected because `(name, kind)` is not a fingerprint of intent. Two callers who
independently want an account named `alice` will almost always both want a
wallet, so the discriminant stays silent in exactly the case that matters and
fires only in the case that was already obviously wrong.

**Answer 201 again** — rejected: 201 asserts a creation that did not occur, and
adds a false status to an already-ambiguous answer.

**Overload `invalid_amount` for malformed bodies** — rejected. A sealed set
whose members are stretched to cover unrelated causes is a set that no longer
tells the caller anything, and the `exhaustive` linter would still pass while the
vocabulary rotted.

**Reject currency mismatch at the adapter as 400** — rejected: the adapter would
have to read both accounts' currencies to decide, which puts a database read in
the driving adapter and duplicates the snapshot the shell already takes under
locks (DDD-6).

**Map currency mismatch to `unbalanced`** — literally true and practically
useless. The caller supplied no entries, so "your entries do not sum to zero"
names nothing they can fix. A refusal must name the thing the caller controls.

**Treat currency mismatch as out of scope and leave it undeclared** — rejected.
Excluding a *capability* does not excuse an undefined *answer*. The currency
column exists (§ Open questions), so two currencies are representable in the
schema today, and DDD-12 makes an undeclared refusal a hard stop.

## Consequences

- Three new wire members: `malformed_request` (400), `account_already_exists`
  (409), `currency_mismatch` (422). Two are new `domain.ViolationKind` members;
  `malformed_request` is adapter-owned and must **not** be added to
  `ViolationKind` — a pure function over a typed command cannot represent "the
  bytes were not JSON", and adding it would put a transport concern inside the
  purity boundary.
- The `exhaustive` linter (CI job 1, DDD-12's compensating control) now has two
  switch surfaces to cover, not one: `domain.ViolationKind` in the core, and the
  wire mapping in the HTTP adapter. Both must be exhaustive, and the adapter's
  switch is the one that keeps the status table honest.
- `POST /accounts` acquires a refusal path it did not have, and therefore a
  status other than 201 for the first time.
- The account table needs a unique constraint on the account name. Expand-only
  migrations allow adding it; it must be created with the table, because adding
  it later against history that already contains duplicates would fail.
- `feature-delta.md` § Driving adapter coverage now has a reachable cause for
  its declared 400. The declared status list for `POST /transfers` gains 401,
  which the auth scenarios already exercised without it being written down.
- Under-declaration remains possible in one place: a transfer whose `from` and
  `to` name the same account is still unspecified. Raised, not decided here.
