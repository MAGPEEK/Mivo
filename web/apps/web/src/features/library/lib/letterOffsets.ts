import type { LetterIndex, LibraryLettersQuery } from "@mivo/api-client";

import type { BrowseListQuery } from "./browseQuery";

// Pure maths of the virtual grid and the A-Z bar (LB-29, LB-30): which letter starts where, and
// how a list position becomes a scroll position. No DOM and no React, so it is table-tested.

/** The bar's entries in display order: "#" for every non-letter start, then A to Z. */
export const alphaBarLetters: readonly string[] = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("")];

/** One button of the A-Z bar. */
export interface AlphaBarEntry {
  readonly letter: string;
  /** Zero-based list position of the letter's first item; `undefined` when no item starts so. */
  readonly offset: number | undefined;
}

/** The bar's entries; a letter the server did not report has no offset and is dimmed. */
export function alphaBarEntries(index: LetterIndex | undefined): readonly AlphaBarEntry[] {
  const offsets = new Map((index?.letters ?? []).map(({ letter, offset }) => [letter, offset]));
  return alphaBarLetters.map((letter) => ({ letter, offset: offsets.get(letter) }));
}

/**
 * The `listLibraryLetters` query of a listing, or `undefined` when the bar does not apply: only
 * the title sort has letters, and the letters endpoint cannot narrow by match state, so its
 * offsets would not fit that list.
 */
export function lettersQueryOf(listQuery: BrowseListQuery): LibraryLettersQuery | undefined {
  const { sort, matchState } = listQuery;
  if ((sort !== undefined && sort !== "title") || matchState !== undefined) {
    return undefined;
  }
  // The title sort has no seed; every other key is shared with the items endpoint.
  return Object.fromEntries(Object.entries(listQuery).filter(([key]) => !notLettersKeys.has(key)));
}

const notLettersKeys: ReadonlySet<string> = new Set(["sort", "seed", "matchState"]);

/** Geometry of a grid: how many columns fit and how wide each one is. */
export interface ColumnLayout {
  readonly columns: number;
  /** Width of one column in CSS pixels; columns stretch to fill the width, like `1fr`. */
  readonly columnWidth: number;
}

/** Widths in CSS pixels that decide the columns of a grid. */
export interface ColumnLayoutInput {
  readonly containerWidth: number;
  /** Narrowest allowed column. */
  readonly minColumnWidth: number;
  readonly gap: number;
}

/** Fits as many columns of at least `minColumnWidth` as the width holds (CSS `auto-fill`). */
export function columnLayout({
  containerWidth,
  minColumnWidth,
  gap,
}: ColumnLayoutInput): ColumnLayout {
  const columns = Math.max(1, Math.floor((containerWidth + gap) / (minColumnWidth + gap)));
  const columnWidth = Math.max(0, (containerWidth - gap * (columns - 1)) / columns);
  return { columns, columnWidth };
}

/** Number of rows that hold `itemCount` items in `columns` columns. */
export function rowCount(itemCount: number, columns: number): number {
  return Math.ceil(itemCount / columns);
}

/** The row a zero-based list position sits in. */
export function rowOfIndex(index: number, columns: number): number {
  return Math.floor(index / columns);
}

/** What {@link scrollTopOfOffset} needs to turn a position into a scroll position. */
export interface ScrollTopInput {
  /** Zero-based list position, e.g. the offset of a letter. */
  readonly offset: number;
  readonly columns: number;
  /** Row height plus the gap below it. */
  readonly rowStride: number;
  /** Distance from the top of the document to the top of the list. */
  readonly listTop: number;
  /** Space kept free above the row, e.g. the sticky top bar. */
  readonly clearance: number;
}

/** Document scroll position that puts the row of `offset` right below the clearance. */
export function scrollTopOfOffset({
  offset,
  columns,
  rowStride,
  listTop,
  clearance,
}: ScrollTopInput): number {
  return Math.max(0, listTop + rowOfIndex(offset, columns) * rowStride - clearance);
}

/**
 * How many items must be loaded before the list position `offset` can be shown: the position
 * plus one. Pages come by cursor, so a jump loads every page up to there.
 */
export function itemsNeededFor(offset: number): number {
  return offset + 1;
}

/** Keys that move the focus in the grid, by position. */
export type GridKey = "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown" | "PageUp" | "PageDown";

/** What {@link focusTarget} needs. */
export interface FocusTargetInput {
  readonly key: GridKey;
  readonly index: number;
  readonly columns: number;
  /** Rows visible in the window; Page Up and Down move by that many. */
  readonly visibleRows: number;
  readonly itemCount: number;
}

/**
 * The position the key moves the focus to, kept inside the list; `undefined` when the key does
 * nothing there (left and right in a single column, where up and down already walk the rows).
 */
export function focusTarget({
  key,
  index,
  columns,
  visibleRows,
  itemCount,
}: FocusTargetInput): number | undefined {
  const horizontal = key === "ArrowLeft" || key === "ArrowRight";
  if (horizontal && columns === 1) {
    return undefined;
  }
  const steps: Record<GridKey, number> = {
    ArrowLeft: -1,
    ArrowRight: 1,
    ArrowUp: -columns,
    ArrowDown: columns,
    PageUp: -columns * Math.max(1, visibleRows),
    PageDown: columns * Math.max(1, visibleRows),
  };
  return Math.min(itemCount - 1, Math.max(0, index + steps[key]));
}
