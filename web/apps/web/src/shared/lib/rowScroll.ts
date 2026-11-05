import { spacing, spacingUnit } from "@mivo/ui-tokens";

/** Shape of the cards in a row: poster 2:3, episode still 16:9, album or photo 1:1. */
export type CardFormat = "poster" | "episode" | "square";

/** Gap between two cards of a row in CSS pixels (the dense web spacing step `lg`). */
export const rowGap = spacing.web.lg;

// Cards keep one width per format, like the prototype's `--card`, so the virtualiser knows
// every card's position without measuring. Episode stills are 1.6 times a poster's width, so
// both rows show artwork of a similar height.
const posterCardUnits = 40;
const episodeCardUnits = 64;

/** Width of one card per format in CSS pixels. */
export const cardWidths: Readonly<Record<CardFormat, number>> = {
  poster: posterCardUnits * spacingUnit,
  episode: episodeCardUnits * spacingUnit,
  square: posterCardUnits * spacingUnit,
};

/** `sizes` attribute of a card image in a row: cards there have a fixed width. */
export function rowCardSizes(format: CardFormat): string {
  return `${String(cardWidths[format])}px`;
}

/** Distance from the start of one card to the start of the next. */
export function cardStride(format: CardFormat): number {
  return cardWidths[format] + rowGap;
}

/** Scroll position and size of a row's scroll container, as the DOM reports them. */
export interface RowGeometry {
  readonly scrollLeft: number;
  readonly clientWidth: number;
  readonly scrollWidth: number;
}

/** Whether a row is scrolled to its start or end; the arrow buttons disable there. */
export interface RowScrollEdges {
  readonly atStart: boolean;
  readonly atEnd: boolean;
}

// Zoomed and high-DPI screens report fractional scroll positions that stop just short of
// the true end.
const edgeTolerancePx = 1;

function maxScrollLeft({ clientWidth, scrollWidth }: RowGeometry): number {
  return Math.max(0, scrollWidth - clientWidth);
}

/** Which ends of the row are reached. */
export function rowScrollEdges(geometry: RowGeometry): RowScrollEdges {
  return {
    atStart: geometry.scrollLeft <= edgeTolerancePx,
    atEnd: geometry.scrollLeft >= maxScrollLeft(geometry) - edgeTolerancePx,
  };
}

/** Direction of an arrow button: -1 is back (previous), 1 is forward (next). */
export type ScrollDirection = -1 | 1;

/**
 * Where an arrow button scrolls to: one page of whole cards (at least one card) in
 * `direction`, clamped to the row, so no card is cut off twice in a row.
 */
export function pageScrollTarget(
  geometry: RowGeometry,
  direction: ScrollDirection,
  stride: number,
): number {
  const wholeCards = Math.max(1, Math.floor(geometry.clientWidth / stride));
  const target = geometry.scrollLeft + direction * wholeCards * stride;
  return Math.min(maxScrollLeft(geometry), Math.max(0, target));
}

/**
 * Index the focus moves to for a key in a row of `count` cards, or `undefined` when the key
 * does not move it. Unlike tabs, rows stop at their ends instead of wrapping.
 */
export function rowKeyTarget(key: string, current: number, count: number): number | undefined {
  const last = count - 1;
  const targets: Readonly<Record<string, number>> = {
    ArrowLeft: current - 1,
    ArrowRight: current + 1,
    Home: 0,
    End: last,
  };
  const target = targets[key];
  if (target === undefined || target < 0 || target > last || target === current) {
    return undefined;
  }
  return target;
}

// A row of this many cards stays small enough to render whole; longer rows only render the
// cards in view, so a library's "Recently added" with hundreds of items stays cheap.
const virtualizeFromCount = 25;

/** Whether a row with `count` cards renders only the visible ones. */
export function shouldVirtualize(count: number): boolean {
  return count >= virtualizeFromCount;
}

/**
 * The rendered card indexes plus the card that holds the row's tab stop, sorted. Keeping that
 * card mounted after it scrolls out of view keeps the row reachable with Tab.
 */
export function withActiveIndex(indexes: readonly number[], active: number): number[] {
  if (indexes.includes(active)) {
    return [...indexes];
  }
  return [...indexes, active].sort((left, right) => left - right);
}
