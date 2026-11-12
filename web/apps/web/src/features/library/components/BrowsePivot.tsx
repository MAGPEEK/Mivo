import type { LibraryKind } from "@mivo/api-client";
import { Fragment, type ReactNode } from "react";

import { useBrowseState, type BrowseSearchChange } from "../hooks/useBrowseState";
import { useLibraryItems, type LibraryItems } from "../hooks/useLibraryItems";
import { useViewPreference } from "../hooks/useViewPreference";
import type { BrowseSearch, BrowseType, LibraryItemFilter } from "../lib/browseQuery";
import { BrowseItemGrid } from "./BrowseItemGrid";
import { BrowseToolbar } from "./BrowseToolbar";
import { FilterChips } from "./FilterChips";

/** Props of {@link BrowsePivot}. */
export interface BrowsePivotProps {
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
  /** What the filters narrow; an empty filtered result needs other texts than an empty library. */
  readonly filter: LibraryItemFilter;
}

/**
 * Renders the listed items (loading, error, empty and loaded states) for a type with its own
 * view, e.g. albums; returns `undefined` for types that use the poster grid.
 */
export type LibraryItemsRenderer = (view: LibraryItemsView) => ReactNode | undefined;

// Key of the listing while a random sort waits for its seed; no request runs meanwhile.
const pendingQueryKey = "pending";

/**
 * The Browse pivot: every item of a library, page by page, with the Browse toolbar above.
 * Albums, books and photos get their own view through `renderItems`, the others the poster grid.
 */
export function BrowsePivot({
  libraryId,
  libraryKind,
  search,
  onSearchChange,
  renderItems,
}: BrowsePivotProps) {
  const browse = useBrowseState({ search, onSearchChange, libraryKind });
  const items = useLibraryItems(libraryId, browse.query);
  // Keyed, so focus targets and "load more" state never carry over to another query.
  const key = browse.query === undefined ? pendingQueryKey : JSON.stringify(browse.query);
  const view = useViewPreference();
  // Types with their own view (albums, photos...) ignore the view style, so no controls.
  const customItems = renderItems?.({ type: browse.type, items, filter: browse.filter });
  const showsViewControls = customItems === undefined;
  return (
    <>
      <BrowseToolbar
        libraryId={libraryId}
        browse={browse}
        totalCount={items.totalCount}
        view={showsViewControls ? view : undefined}
      />
      <FilterChips browse={browse} />
      <Fragment key={key}>
        {customItems ?? <BrowseItemGrid items={items} filter={browse.filter} view={view} />}
      </Fragment>
    </>
  );
}
