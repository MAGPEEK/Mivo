// Whether the next queue item starts by itself (PL-14), kept per device. The next-up logic
// (M5-66) reads it with `readAutoPlay`.

import { productId } from "@mivo/ui-tokens";

/** Auto play is on until the viewer switches it off. */
export const defaultAutoPlay = true;

/** Key of the preference in `localStorage`. */
export const autoPlayStorageKey = `${productId}.player.autoPlay`;

/** Reads the saved choice; the default without storage or for a value this version cannot read. */
export function readAutoPlay(storage: Storage | undefined): boolean {
  try {
    const stored = storage?.getItem(autoPlayStorageKey);
    if (stored === "true") {
      return true;
    }
    return stored === "false" ? false : defaultAutoPlay;
  } catch {
    return defaultAutoPlay;
  }
}

/** Saves the choice; a missing storage or a failed write only costs the memory. */
export function writeAutoPlay(storage: Storage | undefined, enabled: boolean): void {
  try {
    storage?.setItem(autoPlayStorageKey, String(enabled));
  } catch {
    // Deliberately ignored: the choice then lasts for this page load only.
  }
}
