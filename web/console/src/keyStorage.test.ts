// keyStorage -- the operator's pasted API key survives across fetches and a
// page reload, and a rejected key leaves no trace to retry against.
//
// Layer: unit (in-memory, jsdom's real Storage implementation -- no network).
// Mandate 8: every state-mutating step asserts via assertStateDelta with a
// port-exposed universe (the one browser storage slot this module owns).
// Mandate 9: layer 1 -> PBT full for the roundtrip property.
import { describe, it, expect, beforeEach } from "vitest";
import fc from "fast-check";
import { createKeyStorage } from "./keyStorage";
import { assertStateDelta, setTo } from "./testing/stateDelta";
import { STORAGE_KEY } from "./testing/domainTypes";

function captureUniverse(): Record<string, unknown> {
  return { [`localStorage.${STORAGE_KEY}`]: window.localStorage.getItem(STORAGE_KEY) };
}

describe("keyStorage -- the operator's pasted API key survives a page reload", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it.skip("@property any pasted key, once set, is exactly what get() returns next", () => {
    fc.assert(
      fc.property(fc.string({ minLength: 1 }), (pastedKey) => {
        window.localStorage.clear();
        const storage = createKeyStorage();
        const before = captureUniverse();
        storage.set(pastedKey);
        const after = captureUniverse();

        assertStateDelta(before, after, new Set([`localStorage.${STORAGE_KEY}`]), {
          [`localStorage.${STORAGE_KEY}`]: setTo(pastedKey),
        });
        expect(storage.get()).toBe(pastedKey);
      })
    );
  });

  it.skip("@error clearing a rejected key leaves no trace for the next fetch to reuse", () => {
    const storage = createKeyStorage();
    storage.set("a-key-the-api-rejected");
    const before = captureUniverse();

    storage.clear();
    const after = captureUniverse();

    assertStateDelta(before, after, new Set([`localStorage.${STORAGE_KEY}`]), {
      [`localStorage.${STORAGE_KEY}`]: setTo(null),
    });
    expect(storage.get()).toBeNull();
  });

  it.skip("a browser that has never stored a key answers get() with null, not a crash (C1: boundary/empty)", () => {
    const storage = createKeyStorage();
    expect(storage.get()).toBeNull();
  });
});
