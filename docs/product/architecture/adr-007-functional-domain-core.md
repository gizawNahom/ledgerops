# ADR-007 — The domain core is functional: pure decide step, immutable types, sealed violations

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decisions: DDD-12, DDD-13, DDD-14, DDD-15, DDD-16
**Supersedes**: DDD-10 (paradigm OOP), recorded in `feature-delta.md` § Changed Assumptions

## Context

DDD-10 set the paradigm to OOP. It was decided by language default rather than
by the design: `/nw-design` step 4 classifies Go as OOP-native and recommends
OOP accordingly. The design it labelled had already gone the other way — DDD-1
specified a pure domain core with I/O at the edges, and `Clock` and `IDGenerator`
were ports precisely so the core could be deterministic.

The label is not cosmetic. It selects which crafter implements the feature, and
the pure-core playbook — the `nw-fp-hexagonal-architecture`,
`nw-fp-domain-modeling`, and `nw-fp-principles` skills — is declared on
`@nw-functional-software-crafter` and cannot be loaded by `@nw-software-crafter`.
Under DDD-10 the implementation would have been written without access to the
guidance for the architecture it was implementing.

Adopting the functional paradigm in Go raises four questions the FP skills do not
answer for this language, because no `nw-fp-go` skill exists. Left open, each
would be settled ad hoc during DELIVER. They are settled here instead.

## Decision

**The paradigm is functional** (DDD-16). Implementation routes to
`@nw-functional-software-crafter`. Go remains the language — DDD-2 was decided on
concurrency primitives for the I4/I7 race tests, and nothing here disturbs that.

**Violations are a sealed taxonomy** (DDD-12). A single domain violation type
carries a kind discriminant — unbalanced (I1), insufficient funds (I4), unknown
account — and its interface can only be satisfied inside the domain package.
Functions keep the idiomatic `(value, error)` shape, so violations travel through
`errors.As` and map onto HTTP 422 without the adapter inventing its own error
vocabulary.

**Ports are hybrid by arity** (DDD-13). Single-operation ports — `Clock`,
`IDGenerator` — are function types. Ports whose operations must share a
transaction handle — `TransactionRepository`, `AccountRepository`,
`IdempotencyStore` — stay interfaces.

**The posting rulebook is one pure function** (DDD-14). `Post` takes the transfer
command, the already-locked account snapshots, the current time, and a
pre-generated transaction id; it returns the transaction, its entries, and the
balance deltas. No I/O, no clock read, no id generation. The application layer
locks, calls it once, and persists what it returns.

**Immutability is strict** (DDD-15). Unexported fields, smart constructors that
validate on the way in, no mutating methods. Applying a debit returns a new
Account value.

## Alternatives considered

**Keep OOP and write FP-shaped Go anyway.** Costs nothing in documentation and
changes nothing in the architecture, since the design was already pure-core. It
fails on the mechanism: the routing token is the only switch that loads the FP
skills, so the crafter would implement a pure-core design without the pure-core
guidance. Rejected — this was the whole reason the label mattered.

**Generic `Result[T]` instead of a sealed error taxonomy.** Closest to the FP
canon, and makes the railway explicit. Rejected because Go has no method type
parameters: `Map` and `AndThen` must be free functions, so composition never
reads as a chain, and every caller unwraps at the boundary anyway. The ceremony
buys nothing that the sealed set does not already give.

**An outcome union interface — `Posted` | `Rejected` — with no error channel for
business outcomes.** Genuinely attractive: a rejection is a legitimate result,
not a failure, and the 422-versus-500 split becomes structural rather than
conventional. Rejected as the more invasive of two options that both produce a
closed set, since every caller including the HTTP adapter would type-switch on
outcomes rather than handle errors. Worth revisiting if the violation set grows
past a handful of kinds.

**All ports as function types**, per the fp-hexagonal canon. Rejected because
the three repositories expose several operations that must run inside the *same*
database transaction. As loose function types, a caller could wire two of them
to different transactions and the type system would not object — which is
exactly the class of bug DDD-6's lock-ordering rule exists to prevent.

**Per-aggregate pure functions** composed by the application layer, rather than
one whole-posting decide step. Rejected because the ordering of the invariant
checks would move into the effect shell, leaving no single unit that holds the
posting rulebook — and therefore no single target for the property tests.

## Consequences

The I1 and I4 invariants are decided in one pure function, testable with no
database and no mocks. Because time and identity arrive as values, those tests
are deterministic and reproducible by seed. The concurrency suite narrows to what
only real PostgreSQL can show: whether the shell acquires locks correctly.

Exhaustiveness over the violation kinds is not compiler-checked. A linter over
the switch sites is the compensating control; wiring it belongs to DEVOPS.

No `nw-fp-go` skill exists — the installed FP language skills are F#, Haskell,
Scala, Clojure, and Kotlin. The crafter's language detection will find `*.go`,
fail to load a Go-specific FP skill, and fall back to generic FP guidance. The
four decisions above are the mitigation: they pre-settle the places where
generic FP guidance and Go's type system disagree.

Strict immutability allocates a new Account per debit. Irrelevant at this scale,
where postings are bounded by database round-trips.
