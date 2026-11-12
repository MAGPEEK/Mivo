import type { CardFormat } from "../../../shared/lib/rowScroll";
import { cardSize, defaultCardStep, type ViewStyle } from "./viewStyle";

/** What the virtual grid needs to know about the cards it lays out. */
export interface VirtualGridGeometry {
  /** Narrowest column in CSS pixels; columns stretch to fill the width. */
  readonly minColumnWidth: number;
  readonly columnGap: number;
  readonly rowGap: number;
  /** Height of one row for a column of the given width. */
  readonly rowHeightOf: (columnWidth: number) => number;
}

// Cards sit 16 px apart in a row and 24 px apart between rows (the old `gap-x-4 gap-y-6`).
const cardColumnGap = 16;
const cardRowGap = 24;
// Title line, details line and the gaps of a media card under its artwork. The rows are laid
// out by arithmetic, not measured, so this is the one number to adjust with the card design.
const cardCaptionHeight = 56;

// Artwork height per width: poster 2:3, episode still 16:9, album and photo 1:1.
const artworkHeightRatios: Readonly<Record<CardFormat, number>> = {
  poster: 3 / 2,
  episode: 9 / 16,
  square: 1,
};

// List and Details are one column of fixed-height rows: a compact row (padding plus one line)
// and a row around the 72 px wide poster thumbnail (108 px high, padding and border).
const listRowHeight = 44;
const detailsRowHeight = 129;
// No column count: a width no screen reaches keeps the grid at one column.
const singleColumnMinWidth = Number.MAX_SAFE_INTEGER;

function cardGeometry(minColumnWidth: number, format: CardFormat): VirtualGridGeometry {
  return {
    minColumnWidth,
    columnGap: cardColumnGap,
    rowGap: cardRowGap,
    rowHeightOf: (columnWidth) =>
      Math.ceil(columnWidth * artworkHeightRatios[format]) + cardCaptionHeight,
  };
}

function rowGeometry(rowHeight: number): VirtualGridGeometry {
  return {
    minColumnWidth: singleColumnMinWidth,
    columnGap: 0,
    rowGap: 0,
    rowHeightOf: () => rowHeight,
  };
}

/** The view a grid shows: the style and the size slider step. */
export interface GridView {
  readonly viewStyle: ViewStyle;
  readonly cardStep: number;
}

/**
 * The layout of a view style: a card grid whose columns follow the size slider, or one column
 * of fixed-height rows. Without a view (features with their own cards) the grid keeps the
 * default card size.
 */
export function gridGeometryOf(
  view: GridView | undefined,
  format: CardFormat = "poster",
): VirtualGridGeometry {
  if (view?.viewStyle === "list") {
    return rowGeometry(listRowHeight);
  }
  if (view?.viewStyle === "details") {
    return rowGeometry(detailsRowHeight);
  }
  return cardGeometry(cardSize(view?.cardStep ?? defaultCardStep, format).columnWidth, format);
}
