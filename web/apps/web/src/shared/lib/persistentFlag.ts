// Stored values are strings; these two spell a boolean so any other value counts as unset.
const storedTrue = "true";
const storedFalse = "false";

/**
 * Returns the browser's `localStorage`, or `undefined` when the browser blocks it. Merely
 * reading `window.localStorage` throws a `SecurityError` when site data is disabled.
 */
export function browserLocalStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

/**
 * Reads a boolean saved by {@link writeFlag}. Returns `fallback` when there is no storage,
 * the key is missing or holds anything else, or reading fails.
 */
export function readFlag(storage: Storage | undefined, key: string, fallback: boolean): boolean {
  try {
    const stored = storage?.getItem(key);
    if (stored === storedTrue) {
      return true;
    }
    return stored === storedFalse ? false : fallback;
  } catch {
    return fallback;
  }
}

/**
 * Saves a boolean under `key`. A missing storage or a failed write (quota, privacy mode) is
 * ignored: the preference then lasts only for the session, which is acceptable for UI state.
 */
export function writeFlag(storage: Storage | undefined, key: string, value: boolean): void {
  try {
    storage?.setItem(key, value ? storedTrue : storedFalse);
  } catch {
    // Deliberately ignored, see above.
  }
}
