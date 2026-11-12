import type { LetterIndex } from "@mivo/api-client";
import { useRef, type CSSProperties, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import type { CardFormat } from "../../../shared/lib/rowScroll";
import {
  cellIndexAttribute,
  useFocusRequest,
  useGridFocus,
  type GridListProps,
} from "../hooks/useGridFocus";
import type { LibraryItems } from "../hooks/useLibraryItems";
import { useScrollMemory } from "../hooks/useScrollMemory";
import { useVirtualItems, type VirtualCell, type VirtualItems } from "../hooks/useVirtualItems";
import { gridGeometryOf, type GridView } from "../lib/gridGeometry";
import { alphaBarEntries } from "../lib/letterOffsets";
import { cardSize } from "../lib/viewStyle";
import { libraryNamespace } from "../locales";
import { AlphaJumpBar } from "./AlphaJumpBar";
import type { BrowseCardProps } from "./browseCardProps";
import { secondaryButtonClass } from "./buttonClasses";
import {
  DetailsRowBody,
  detailsRowClass,
  ListRowBody,
  listRowClass,
  type RowBodyProps,
} from "./ItemRows";

/** Props of {@link VirtualItemGrid}. */
export interface VirtualItemGridProps {
  readonly items: LibraryItems;
  /** Accessible name of the list. */
  readonly label: string;
  /** Text shown when the next page failed to load. */
  readonly loadMoreFailedText: string;
  /** Text read out while the next page loads. */
  readonly loadingMoreText: string;
  readonly renderCard: (card: BrowseCardProps) => ReactNode;
  /** View style and card size; without it the grid keeps the default card size. */
  readonly view: GridView | undefined;
  readonly cardFormat: CardFormat | undefined;
  /** Where each letter starts; the A-Z bar shows only with it. */
  readonly letters: LetterIndex | undefined;
  /** Identifies the listing for the scroll position; `undefined` keeps no position. */
  readonly scrollKey: string | undefined;
}

function cellStyle({ left, top, width, height }: VirtualCell): CSSProperties {
  return { position: "absolute", left, top, width, height };
}

interface CellProps {
  readonly cell: VirtualCell;
  readonly view: GridView | undefined;
  readonly renderCard: VirtualItemGridProps["renderCard"];
  readonly cardFormat: CardFormat | undefined;
  readonly focusOnMount: boolean;
  readonly itemCount: number;
  /** Items in the whole listing, for assistive technology; unknown when not counted. */
  readonly setSize: number | undefined;
}

function CellBody({ item, view, renderCard, cardFormat, focusOnMount }: CellProps & RowBodyProps) {
  if (view?.viewStyle === "list") {
    return <ListRowBody item={item} focusOnMount={focusOnMount} />;
  }
  if (view?.viewStyle === "details") {
    return <DetailsRowBody item={item} focusOnMount={focusOnMount} />;
  }
  const size = view === undefined ? undefined : cardSize(view.cardStep, cardFormat);
  return renderCard({ item, focusOnMount, size });
}

function cellClass(view: GridView | undefined): string {
  if (view?.viewStyle === "list") {
    return listRowClass;
  }
  return view?.viewStyle === "details" ? detailsRowClass : "";
}

// A card whose page is not loaded yet: a pulsing box of the card's size, hidden from
// assistive technology like the grid's loading state, which `aria-busy` announces.
function SkeletonCell({ cell, view }: Pick<CellProps, "cell" | "view">) {
  return (
    <li
      aria-hidden="true"
      {...{ [cellIndexAttribute]: cell.index }}
      style={cellStyle(cell)}
      className={view?.viewStyle === "grid" || view === undefined ? "" : "border-b border-divider"}
    >
      <div className="size-full animate-pulse rounded-md bg-surface-raised motion-reduce:animate-none" />
    </li>
  );
}

function Cell(props: CellProps) {
  const { cell, view, itemCount } = props;
  if (cell.item === undefined || cell.index >= itemCount) {
    return <SkeletonCell cell={cell} view={view} />;
  }
  return (
    <li
      {...{ [cellIndexAttribute]: cell.index }}
      aria-posinset={cell.index + 1}
      aria-setsize={props.setSize}
      style={cellStyle(cell)}
      className={`scroll-mt-20 ${cellClass(view)}`}
    >
      <CellBody {...props} item={cell.item} />
    </li>
  );
}

interface CellListProps {
  readonly grid: VirtualItems;
  readonly items: LibraryItems;
  readonly cellProps: Omit<CellProps, "cell" | "itemCount" | "setSize" | "focusOnMount">;
  readonly listProps: GridListProps;
  readonly label: string;
  readonly listRef: RefObject<HTMLUListElement | null>;
}

// One absolutely positioned cell per mounted card, inside a list as tall as the whole listing.
function CellList({ grid, items, cellProps, listProps, label, listRef }: CellListProps) {
  return (
    // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
    <ul
      ref={listRef}
      role="list"
      aria-label={label}
      aria-busy={items.loadingMore}
      style={{ position: "relative", height: grid.totalHeight }}
      className="min-w-0 flex-1"
      {...listProps}
    >
      {grid.cells.map((cell) => (
        <Cell
          key={cell.index}
          {...cellProps}
          cell={cell}
          focusOnMount={false}
          itemCount={items.items.length}
          setSize={items.totalCount}
        />
      ))}
    </ul>
  );
}

/**
 * The listing of a library as a virtual grid (LB-29, LB-30): only the rows near the window are
 * in the DOM, pages load as the rows come into view, unloaded cards are skeletons, arrow keys
 * move the focus and the A-Z bar jumps to a letter. Grid, List and Details share it; they
 * differ in the geometry of a row and in the content of a cell.
 */
export function VirtualItemGrid(props: VirtualItemGridProps) {
  const { items, view, cardFormat, letters, scrollKey } = props;
  const listRef = useRef<HTMLUListElement>(null);
  const focus = useGridFocus();
  const grid = useVirtualItems({
    listRef,
    items,
    geometry: gridGeometryOf(view, cardFormat),
    pinnedIndex: focus.pinnedIndex,
  });
  useFocusRequest({
    listRef,
    request: focus.request,
    clearRequest: focus.clearRequest,
    mountedKey: grid.mountedKey,
  });
  useScrollMemory({ scrollKey, ready: grid.ready });
  const jumpToOffset = (offset: number) => {
    grid.scrollToIndex(offset);
    focus.requestFocus(offset, { scroll: false });
  };
  const listProps = focus.listProps({
    columns: grid.columns,
    visibleRows: grid.visibleRows,
    itemCount: items.items.length,
  });
  return (
    <>
      <div className="flex items-start gap-3">
        <CellList
          grid={grid}
          items={items}
          cellProps={{ view, cardFormat, renderCard: props.renderCard }}
          listProps={listProps}
          label={props.label}
          listRef={listRef}
        />
        {letters !== undefined && (
          <AlphaJumpBar entries={alphaBarEntries(letters)} onJump={jumpToOffset} />
        )}
      </div>
      <LoadMoreStatus
        items={items}
        failedText={props.loadMoreFailedText}
        busyText={props.loadingMoreText}
      />
    </>
  );
}

interface LoadMoreStatusProps {
  readonly items: LibraryItems;
  readonly failedText: string;
  readonly busyText: string;
}

// Pages load by themselves; this only speaks when one is loading or has failed.
function LoadMoreStatus({ items, failedText, busyText }: LoadMoreStatusProps) {
  const { t } = useTranslation(libraryNamespace);
  if (items.loadMoreFailed && !items.loadingMore) {
    return (
      <div className="mt-6 flex flex-col items-center gap-3">
        <p role="alert" className="text-sm text-text-muted">
          {failedText}
        </p>
        <button type="button" onClick={items.loadMore} className={secondaryButtonClass}>
          {t("retry")}
        </button>
      </div>
    );
  }
  return (
    <p role="status" className="sr-only">
      {items.loadingMore ? busyText : ""}
    </p>
  );
}
