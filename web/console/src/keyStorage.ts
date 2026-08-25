// keyStorage -- the sole module permitted to touch window.localStorage
// (DESIGN SA-D4, Core Principle 12 capability injection).
import { STORAGE_KEY } from "./testing/domainTypes";

export interface KeyStorage {
  get(): string | null;
  set(key: string): void;
  clear(): void;
}

export function createKeyStorage(): KeyStorage {
  return {
    get(): string | null {
      return window.localStorage.getItem(STORAGE_KEY);
    },
    set(key: string): void {
      window.localStorage.setItem(STORAGE_KEY, key);
    },
    clear(): void {
      window.localStorage.removeItem(STORAGE_KEY);
    },
  };
}

// Exported for scaffold-detection tooling and for tests that assert against
// the exact storage key name DESIGN settled on.
export { STORAGE_KEY };
