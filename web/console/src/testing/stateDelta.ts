// TypeScript port of the Universe-bound state-delta assertion contract
// (nw-test-design-mandates Mandate 8). Bootstrap for this project's FIRST
// TypeScript DISTILL run (ledger-core-console, 2026-08-25).
//
// Canonical pilot is Python (nwave_ai/state_delta). The Polyglot Adapter
// Matrix's stated path convention is `tests/common/state_delta.<ext>`
// (project-local, shared across the whole repo). This project is a Go
// monorepo where `tests/` at the root is the Go acceptance-test root and
// `web/console/` is a *separate* npm package with its own module resolution
// boundary -- importing across that boundary from a root-level `tests/common/`
// is not resolvable without a workspace/monorepo tooling change DESIGN never
// authorized. DISTILL decision (recorded in feature-delta.md Wave: DISTILL /
// [REF] Toolchain): the TS port lives inside the npm package at
// `web/console/src/testing/stateDelta.ts` instead. This is a documented
// deviation from the literal path, not a silent one.
//
// Predicate coverage: this bootstrap implements only the two predicates this
// feature's tests actually need (setTo, unchanged). The full eight-predicate
// library (appendedWith, containing, normalizedTo, idempotentAfter,
// legacyHealed, prependedWith, ...) is a documented gap, deferred to the next
// TS feature that needs them -- see feature-delta.md Self-Completeness Audit.

export type Universe = Record<string, unknown>;
export type Predicate = (before: unknown, after: unknown) => boolean;

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

export function setTo(expected: unknown): Predicate {
  return (_before, after) => deepEqual(after, expected);
}

export function unchanged(): Predicate {
  return (before, after) => deepEqual(before, after);
}

/**
 * Asserts that every key in `universe` changed exactly as `expected`
 * declares (default: unchanged), and that nothing outside `universe` mutated
 * (fail-closed). `before`/`after` are flat maps keyed by port-exposed
 * observable name -- never an internal field name.
 */
export function assertStateDelta(
  before: Universe,
  after: Universe,
  universe: Set<string>,
  expected: Partial<Record<string, Predicate>>
): void {
  for (const key of universe) {
    const predicate = expected[key] ?? unchanged();
    if (!predicate(before[key], after[key])) {
      throw new Error(
        `state-delta violation on universe key "${key}": before=${JSON.stringify(
          before[key]
        )} after=${JSON.stringify(after[key])}`
      );
    }
  }
  const declaredKeys = new Set(universe);
  const leaked = Object.keys(after).filter(
    (k) => !declaredKeys.has(k) && !deepEqual(before[k], after[k])
  );
  if (leaked.length > 0) {
    throw new Error(
      `state-delta violation: mutation outside declared universe: ${leaked.join(", ")}`
    );
  }
}
