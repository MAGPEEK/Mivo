import { productId } from "@mivo/ui-tokens";

// The scroll position of the last listing, so Back from an item page lands where the user left.
// One slot: a listing under another key (a new filter or sort, another library) starts at the
// top, which is also what a changed filter must do.

/** `sessionStorage` key of the remembered position, derived from the brand id (ADR 0002). */
export const scrollMemoryStorageKey = `${productId}.library.scroll`;

/** The remembered position of one listing. */
export interface RememberedScroll {
  /** Identifies the listing: library, query and view. */
  readonly key: string;
  /** Document scroll position in CSS pixels. */
  readonly top: number;
}

/** `sessionStorage`, or `undefined` when the browser blocks it. */
export function browserSessionStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

/** The remembered scroll position of the listing `key`; `undefined` for another or none. */
export function readScroll(storage: Storage | undefined, key: string): number | undefined {
  try {
    const stored: unknown = JSON.parse(storage?.getItem(scrollMemoryStorageKey) ?? "null");
    if (typeof stored !== "object" || stored === null) {
      return undefined;
    }
    const { key: storedKey, top } = stored as Partial<RememberedScroll>;
    const valid = typeof top === "number" && Number.isFinite(top) && top >= 0;
    return storedKey === key && valid ? top : undefined;
  } catch {
    return undefined;
  }
}

/** Remembers the position; a missing storage or a failed write only costs the restore. */
export function writeScroll(storage: Storage | undefined, remembered: RememberedScroll): void {
  try {
    storage?.setItem(scrollMemoryStorageKey, JSON.stringify(remembered));
  } catch {
    // Deliberately ignored: Back then opens the listing at the top.
  }
}
