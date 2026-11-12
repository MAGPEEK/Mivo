import type { ItemSummary } from "@mivo/api-client";
import {
  defaultRangeExtractor,
  useWindowVirtualizer,
  type Range,
  type Virtualizer,
} from "@tanstack/react-virtual";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";

import {
  columnLayout,
  itemsNeededFor,
  rowCount,
  rowOfIndex,
  scrollTopOfOffset,
} from "../lib/letterOffsets";
import type { VirtualGridGeometry } from "../lib/gridGeometry";
import { useElementWidth } from "./useElementWidth";
import type { LibraryItems } from "./useLibraryItems";

/** Options of {@link useVirtualItems}. */
export interface VirtualItemsOptions {
  /** The list element; its width sets the columns and its position the scroll offset. */
  readonly listRef: RefObject<HTMLUListElement | null>;
  readonly items: LibraryItems;
  readonly geometry: VirtualGridGeometry;
  /** A row that must stay in the DOM, e.g. the one that holds the keyboard focus. */
  readonly pinnedIndex: number | undefined;
}

/** One card slot of the grid: an item, or a skeleton while its page is not loaded. */
export interface VirtualCell {
  readonly index: number;
  readonly item: ItemSummary | undefined;
  /** Position inside the list, in CSS pixels. */
  readonly left: number;
  readonly top: number;
  readonly width: number;
  readonly height: number;
}

/** The rows in the DOM and the controls of a virtual grid. */
export interface VirtualItems {
  readonly cells: readonly VirtualCell[];
  /** Height of the whole list, so the page scrolls as if every row was there. */
  readonly totalHeight: number;
  readonly columns: number;
  /** Whole rows that fit the window; Page Up and Down move by that many. */
  readonly visibleRows: number;
  /** Changes whenever other cells are mounted; see `useFocusRequest`. */
  readonly mountedKey: string;
  /** Whether the grid has measured its width and laid out its columns. */
  readonly ready: boolean;
  /** Scrolls the row of `index` to the top, loading pages up to it first. */
  readonly scrollToIndex: (index: number) => void;
}

// Rows above and below the window that stay mounted, so scrolling shows no blank rows.
const overscanRows = 3;
// Rows below the last visible one whose items are requested ahead of the scroll.
const prefetchRows = 2;
// The sticky top bar (3.5 rem) plus air: the row a jump scrolls to must not hide below it.
const stickyTopClearance = 72;

function totalItemCount(items: LibraryItems, columns: number): number {
  const loaded = items.items.length;
  // Without a count the next page is unknown: one row of skeletons stands for it.
  return Math.max(loaded, items.totalCount ?? loaded + (items.hasMore ? columns : 0));
}

// The rows to mount: the window with its overscan, plus the pinned row.
function extractWithPinned(pinnedRow: number | undefined) {
  return (range: Range): number[] => {
    const rows = defaultRangeExtractor(range);
    if (pinnedRow === undefined || pinnedRow >= range.count || rows.includes(pinnedRow)) {
      return rows;
    }
    return [...rows, pinnedRow].sort((left, right) => left - right);
  };
}

interface DemandOptions {
  readonly items: LibraryItems;
  /** Items that must be loaded for what is on screen or was jumped to. */
  readonly wantedCount: number;
}

// Pages come by cursor, so reaching further means loading every page on the way, one at a time.
// A failed page stops the chain: the grid offers a retry instead of looping.
function useDemandLoading({ items, wantedCount }: DemandOptions): void {
  const { loadMore, hasMore, loadingMore, loadMoreFailed } = items;
  const loaded = items.items.length;
  useEffect(() => {
    if (loaded < wantedCount && hasMore && !loadingMore && !loadMoreFailed) {
      loadMore();
    }
  }, [loaded, wantedCount, hasMore, loadingMore, loadMoreFailed, loadMore]);
}

function useListTop(listRef: RefObject<HTMLElement | null>, width: number): number {
  const [listTop, setListTop] = useState(0);
  useLayoutEffect(() => {
    const element = listRef.current;
    if (element !== null) {
      setListTop(element.getBoundingClientRect().top + window.scrollY);
    }
  }, [listRef, width]);
  return listTop;
}

function mountedCells(
  virtualizer: Virtualizer<Window, Element>,
  grid: GridState,
  items: LibraryItems,
): readonly VirtualCell[] {
  const { columns, columnWidth, rowStride, rowHeight, columnGap, total } = grid;
  return virtualizer.getVirtualItems().flatMap(({ index: row }) => {
    const first = row * columns;
    const last = Math.min(total, first + columns);
    return Array.from({ length: last - first }, (_unused, column) => ({
      index: first + column,
      item: items.items[first + column],
      left: column * (columnWidth + columnGap),
      top: row * rowStride,
      width: columnWidth,
      height: rowHeight,
    }));
  });
}

interface GridState {
  readonly columns: number;
  readonly columnWidth: number;
  readonly rowStride: number;
  readonly rowHeight: number;
  readonly columnGap: number;
  readonly rowGap: number;
  readonly total: number;
}

function gridStateOf(items: LibraryItems, geometry: VirtualGridGeometry, width: number) {
  const { minColumnWidth, columnGap, rowGap, rowHeightOf } = geometry;
  const { columns, columnWidth } = columnLayout({
    containerWidth: width,
    minColumnWidth,
    gap: columnGap,
  });
  const rowHeight = rowHeightOf(columnWidth);
  return {
    columns,
    columnWidth,
    rowStride: rowHeight + rowGap,
    rowHeight,
    columnGap,
    rowGap,
    total: totalItemCount(items, columns),
  } satisfies GridState;
}

/**
 * Lays a library listing out as rows of cards and mounts only the rows near the window (LB-29):
 * the page scrolls as the document does, the list is as tall as every row together, and the
 * pages of items are requested as the rows come into view. Heights are computed, not measured,
 * so a row is where the arithmetic puts it and a jump to a letter is exact.
 */
export function useVirtualItems({
  listRef,
  items,
  geometry,
  pinnedIndex,
}: VirtualItemsOptions): VirtualItems {
  const width = useElementWidth(listRef);
  const grid = gridStateOf(items, geometry, width);
  const { columns, rowStride, total } = grid;
  const listTop = useListTop(listRef, width);
  const rows = width === 0 ? 0 : rowCount(total, columns);
  const pinnedRow = pinnedIndex === undefined ? undefined : rowOfIndex(pinnedIndex, columns);
  const virtualizer = useWindowVirtualizer({
    count: rows,
    estimateSize: () => rowStride,
    overscan: overscanRows,
    scrollMargin: listTop,
    rangeExtractor: extractWithPinned(pinnedRow),
  });
  // The size cache does not follow a new estimate, which a width or card size change brings.
  useLayoutEffect(() => {
    virtualizer.measure();
  }, [virtualizer, rowStride]);
  const lastRow = virtualizer.getVirtualItems().at(-1)?.index ?? 0;
  const { scrollToIndex, wantedCount } = useJump({ items, grid, listTop, lastRow });
  useDemandLoading({ items, wantedCount });
  const cells = width === 0 ? [] : mountedCells(virtualizer, grid, items);
  return {
    cells,
    mountedKey: `${String(cells[0]?.index)}-${String(cells.at(-1)?.index)}-${String(items.items.length)}`,
    totalHeight: Math.max(0, rows * rowStride - grid.rowGap),
    columns,
    visibleRows: Math.max(1, Math.floor((virtualizer.scrollRect?.height ?? 0) / rowStride)),
    ready: width > 0,
    scrollToIndex,
  };
}

interface JumpOptions {
  readonly items: LibraryItems;
  readonly grid: GridState;
  readonly listTop: number;
  readonly lastRow: number;
}

// A jump is an object, so jumping to the same position again is a new jump.
interface Jump {
  readonly offset: number;
}

function useJump({ items, grid, listTop, lastRow }: JumpOptions) {
  const [jump, setJump] = useState<Jump>();
  const handled = useRef<Jump | undefined>(undefined);
  const { columns, rowStride } = grid;
  const loaded = items.items.length;
  useEffect(() => {
    if (jump === undefined || handled.current === jump || loaded < itemsNeededFor(jump.offset)) {
      return;
    }
    handled.current = jump;
    window.scrollTo({
      top: scrollTopOfOffset({
        offset: jump.offset,
        columns,
        rowStride,
        listTop,
        clearance: stickyTopClearance,
      }),
    });
  }, [jump, loaded, columns, rowStride, listTop]);
  const onScreenCount = Math.min(grid.total, (lastRow + 1 + prefetchRows) * columns);
  const jumpCount = jump === undefined ? 0 : itemsNeededFor(jump.offset);
  const scrollToIndex = useCallback((offset: number) => {
    setJump({ offset });
  }, []);
  return { scrollToIndex, wantedCount: Math.max(onScreenCount, jumpCount) };
}
