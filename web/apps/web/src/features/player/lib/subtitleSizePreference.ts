// The size of the subtitles the player draws itself (PL-13), kept per device.

import { productId } from "@mivo/ui-tokens";

/** The sizes the viewer can choose, smallest first. */
export const subtitleSizes = ["small", "normal", "large", "huge"] as const;

/** One of {@link subtitleSizes}. */
export type SubtitleSize = (typeof subtitleSizes)[number];

/** The size nothing was saved for. */
export const defaultSubtitleSize: SubtitleSize = "normal";

/** Key of the preference in `localStorage`. */
export const subtitleSizeStorageKey = `${productId}.player.subtitleSize`;

/** Whether `value` names a size. */
export function isSubtitleSize(value: unknown): value is SubtitleSize {
  return subtitleSizes.some((size) => size === value);
}

/** Reads the saved size; the default without storage or for a value this version cannot read. */
export function readSubtitleSize(storage: Storage | undefined): SubtitleSize {
  try {
    const stored = storage?.getItem(subtitleSizeStorageKey);
    return isSubtitleSize(stored) ? stored : defaultSubtitleSize;
  } catch {
    return defaultSubtitleSize;
  }
}

/** Saves the size; a missing storage or a failed write only costs the memory. */
export function writeSubtitleSize(storage: Storage | undefined, size: SubtitleSize): void {
  try {
    storage?.setItem(subtitleSizeStorageKey, size);
  } catch {
    // Deliberately ignored: the size then lasts for this page load only.
  }
}

// Tailwind steps of the cue text; the layer scales with the viewport so a phone and a TV both
// read well. The numbers are the ratios of the approved prototype (16, 22, 30, 40 px at 1080p).
const sizeClasses: Readonly<Record<SubtitleSize, string>> = {
  small: "text-sm md:text-base",
  normal: "text-base md:text-xl",
  large: "text-xl md:text-3xl",
  huge: "text-2xl md:text-5xl",
};

/** The text size classes of the cue layer for `size`. */
export function subtitleSizeClass(size: SubtitleSize): string {
  return sizeClasses[size];
}
