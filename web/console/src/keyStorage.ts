// keyStorage -- the sole module permitted to touch window.localStorage
// (DESIGN SA-D4, Core Principle 12 capability injection). RED scaffold
// created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

import { STORAGE_KEY } from "./testing/domainTypes";

export interface KeyStorage {
  get(): string | null;
  set(key: string): void;
  clear(): void;
}

export function createKeyStorage(): KeyStorage {
  return {
    get(): string | null {
      throw new Error("Not yet implemented -- RED scaffold");
    },
    set(_key: string): void {
      throw new Error("Not yet implemented -- RED scaffold");
    },
    clear(): void {
      throw new Error("Not yet implemented -- RED scaffold");
    },
  };
}

// Exported for scaffold-detection tooling and for tests that assert against
// the exact storage key name DESIGN settled on.
export { STORAGE_KEY };
