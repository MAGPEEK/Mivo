import { productId } from "@mivo/ui-tokens";

import { libraryPivots, type LibraryPivot } from "./browseQuery";

// The last used pivot of every library is kept per device (task M5-30), like the other UI
// preferences in `localStorage`. It is a convenience: without storage the page opens Browse.

/** The pivot a library opens on when none is remembered: the full listing (owner Q6). */
export const defaultPivot: LibraryPivot = "browse";

/** `localStorage` key of the remembered pivot of a library. Derived from the brand id (ADR 0002). */
export function pivotStorageKey(libraryId: string): string {
  return `${productId}.library.${libraryId}.pivot`;
}

/** The pivot remembered for the library, or `undefined` when none (or an unknown value) is stored. */
export function readRememberedPivot(
  storage: Storage | undefined,
  libraryId: string,
): LibraryPivot | undefined {
  try {
    const stored = storage?.getItem(pivotStorageKey(libraryId));
    return libraryPivots.find((pivot) => pivot === stored);
  } catch {
    return undefined;
  }
}

/**
 * Remembers the pivot of a library. A missing storage or a failed write (quota, privacy mode)
 * is ignored: the pivot then lasts only for the session, which is acceptable for UI state.
 */
export function rememberPivot(
  storage: Storage | undefined,
  libraryId: string,
  pivot: LibraryPivot,
): void {
  try {
    storage?.setItem(pivotStorageKey(libraryId), pivot);
  } catch {
    // Deliberately ignored, see above.
  }
}

/** The pivot to show: the one in the URL, else the remembered one, else {@link defaultPivot}. */
export function resolvePivot(
  requested: LibraryPivot | undefined,
  remembered: LibraryPivot | undefined,
): LibraryPivot {
  return requested ?? remembered ?? defaultPivot;
}
