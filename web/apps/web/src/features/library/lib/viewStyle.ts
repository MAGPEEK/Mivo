import { imageWidths, type ImageWidth } from "@mivo/api-client";
import { productId } from "@mivo/ui-tokens";

import type { CardFormat } from "../../../shared/lib/rowScroll";

// The view style and the card size are kept per device (LB-27, LB-28), like the pivot, and not
// in the URL: they describe how this screen likes to look, not what is listed.

/** How a list of items is laid out: poster grid, compact rows or rows with a thumbnail. */
export const viewStyles = ["grid", "list", "details"] as const;

/** One of {@link viewStyles}. */
export type ViewStyle = (typeof viewStyles)[number];

/** The view of a library that never chose one. */
export const defaultViewStyle: ViewStyle = "grid";

/** Card widths in CSS pixels of the five slider steps, smallest first (as in `browse.html`). */
export const cardColumnWidths = [120, 140, 160, 190, 230] as const;

/** Number of slider steps. */
export const cardStepCount = cardColumnWidths.length;

/** The slider step of a device that never chose one (160 px, like the old fixed grid). */
export const defaultCardStep = 2;

// Wide cards (episode stills, 16:9) get a wider column so their height stays like a poster's.
const wideFormatFactor = 1.6;
// Retina screens ask for twice the CSS width; the server only renders the fixed ImageWidth set.
const pixelRatio = 2;

/** What a card of one slider step needs: its column width and the image it asks for. */
export interface CardSize {
  /** Minimum column width of the grid in CSS pixels. */
  readonly columnWidth: number;
  /** `sizes` attribute for the poster: the displayed width. */
  readonly sizes: string;
  /** The smallest allowed rendition that is sharp at pixel ratio 2, for the plain `src`. */
  readonly imageWidth: ImageWidth;
}

/** Clamps `step` into the slider range; non-integers and NaN fall back to the default step. */
export function clampCardStep(step: number): number {
  if (!Number.isInteger(step)) {
    return defaultCardStep;
  }
  return Math.min(cardStepCount - 1, Math.max(0, step));
}

/** The smallest allowed image width of at least `pixels`; the largest one when none is. */
export function imageWidthFor(pixels: number): ImageWidth {
  const fitting = imageWidths.find((width) => width >= pixels);
  return fitting ?? imageWidths[imageWidths.length - 1] ?? 1920;
}

/** The size of the cards in `step` for a card `format`. Episode cards are wide, the rest not. */
export function cardSize(step: number, format: CardFormat = "poster"): CardSize {
  const base = cardColumnWidths[clampCardStep(step)] ?? cardColumnWidths[defaultCardStep];
  const columnWidth = format === "episode" ? Math.round(base * wideFormatFactor) : base;
  return {
    columnWidth,
    sizes: `${String(columnWidth)}px`,
    imageWidth: imageWidthFor(columnWidth * pixelRatio),
  };
}

/** `localStorage` key of the view style, derived from the brand id (ADR 0002). */
export const viewStyleStorageKey = `${productId}.library.viewStyle`;

/** `localStorage` key of the card size step. */
export const cardStepStorageKey = `${productId}.library.cardStep`;

/** The stored view style, or the default for a missing, unknown or unreadable value. */
export function readViewStyle(storage: Storage | undefined): ViewStyle {
  try {
    const stored = storage?.getItem(viewStyleStorageKey);
    return viewStyles.find((style) => style === stored) ?? defaultViewStyle;
  } catch {
    return defaultViewStyle;
  }
}

/** The stored slider step, or the default for a missing or invalid value. */
export function readCardStep(storage: Storage | undefined): number {
  try {
    const stored = storage?.getItem(cardStepStorageKey);
    if (stored === null || stored === undefined || !/^\d+$/.test(stored)) {
      return defaultCardStep;
    }
    const step = Number(stored);
    return step < cardStepCount ? step : defaultCardStep;
  } catch {
    return defaultCardStep;
  }
}

/** Saves a preference; a missing storage or a failed write only costs the persistence. */
export function writePreference(storage: Storage | undefined, key: string, value: string): void {
  try {
    storage?.setItem(key, value);
  } catch {
    // Deliberately ignored: the choice then lasts for the session, fine for UI state.
  }
}
