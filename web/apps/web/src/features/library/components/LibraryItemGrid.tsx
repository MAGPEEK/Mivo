import type { LibraryKind } from "@mivo/api-client";
import { Fragment, type ReactNode } from "react";

import { useBrowseState, type BrowseSearchChange } from "../hooks/useBrowseState";
import { useLibraryItems, type LibraryItems } from "../hooks/useLibraryItems";
import type { BrowseSearch, BrowseType, LibraryItemFilter } from "../lib/browseQuery";
import { BrowseItemGrid } from "./BrowseItemGrid";
import { BrowseToolbar } from "./BrowseToolbar";

/** Props of {@link LibraryItemGrid}. */
export interface LibraryItemGridProps {
  readonly libraryId: string;
  readonly libraryKind: LibraryKind;
  /** Sort, type and filter from the URL. */
  readonly search: BrowseSearch;
  readonly onSearchChange: BrowseSearchChange;
  /** Renders the items of a type with its own view; the poster grid shows the others. */
  readonly renderItems?: LibraryItemsRenderer | undefined;
}

/** What a {@link LibraryItemsRenderer} gets: the listed type, its loaded pages and the filter. */
export interface LibraryItemsView {
  readonly type: BrowseType;
  readonly items: LibraryItems;
  /** The toolbar filter; an empty Unmatched result needs other texts than an empty library. */
  readonly filter: LibraryItemFilter;
}

/**
 * Renders the listed items (loading, error, empty and loaded states) for a type with its own
 * view, e.g. albums; returns `undefined` for types that use the poster grid.
 */
export type LibraryItemsRenderer = (view: LibraryItemsView) => ReactNode | undefined;

/** The items of a library as a poster grid, page by page, with the Browse toolbar above. */
export function LibraryItemGrid({
  libraryId,
  libraryKind,
  search,
  onSearchChange,
  renderItems,
}: LibraryItemGridProps) {
  const browse = useBrowseState({ search, onSearchChange, libraryKind });
  const items = useLibraryItems(libraryId, browse.query);
  // Keyed, so focus targets and "load more" state never carry over to another query.
  const key = JSON.stringify(browse.query);
  return (
    <>
      <BrowseToolbar browse={browse} totalCount={items.totalCount} />
      <Fragment key={key}>
        {renderItems?.({ type: browse.type, items, filter: browse.filter }) ?? (
          <BrowseItemGrid items={items} filter={browse.filter} />
        )}
      </Fragment>
    </>
  );
}
