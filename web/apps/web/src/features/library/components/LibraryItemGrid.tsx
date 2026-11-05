import type { LibraryKind } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { EmptyState, LibraryIcon, Skeleton } from "../../../shared/ui";
import { useBrowseState, type BrowseSearchChange } from "../hooks/useBrowseState";
import { useLibraryItems, type LibraryItems } from "../hooks/useLibraryItems";
import { useNewPageFocus } from "../hooks/useNewPageFocus";
import type { BrowseSearch, LibraryItemFilter } from "../lib/browseQuery";
import { libraryNamespace } from "../locales";
import { BrowseToolbar } from "./BrowseToolbar";
import { secondaryButtonClass } from "./buttonClasses";
import { itemGridClass } from "./gridLayout";
import { ItemCard } from "./ItemCard";

/** Props of {@link LibraryItemGrid}. */
export interface LibraryItemGridProps {
  readonly libraryId: string;
  readonly libraryKind: LibraryKind;
  /** Sort, type and filter from the URL. */
  readonly search: BrowseSearch;
  readonly onSearchChange: BrowseSearchChange;
}

/** The items of a library as a poster grid, page by page, with the Browse toolbar above. */
export function LibraryItemGrid({
  libraryId,
  libraryKind,
  search,
  onSearchChange,
}: LibraryItemGridProps) {
  const browse = useBrowseState({ search, onSearchChange, libraryKind });
  const items = useLibraryItems(libraryId, browse.query);
  return (
    <>
      <BrowseToolbar browse={browse} totalCount={items.totalCount} />
      {/* Keyed, so focus targets and "load more" state never carry over to another query. */}
      <BrowseItems key={JSON.stringify(browse.query)} items={items} filter={browse.filter} />
    </>
  );
}

interface BrowseItemsProps {
  readonly items: LibraryItems;
  readonly filter: LibraryItemFilter;
}

function BrowseItems({ items, filter }: BrowseItemsProps) {
  const { t } = useTranslation(libraryNamespace);
  if (items.status === "pending") {
    return (
      <div role="status" className="w-40">
        <Skeleton label={t("items.loading")} className="aspect-2/3 w-full rounded-md" />
      </div>
    );
  }
  if (items.status === "error") {
    return <ItemsLoadError items={items} />;
  }
  if (items.items.length === 0) {
    const emptyKey = filter === "unmatched" ? "items.emptyUnmatched" : "items.empty";
    return (
      <EmptyState
        icon={<LibraryIcon className="size-10" />}
        title={t(`${emptyKey}.title`)}
        description={t(`${emptyKey}.hint`)}
      />
    );
  }
  return <ItemList items={items} />;
}

function ItemsLoadError({ items }: { readonly items: LibraryItems }) {
  const { t } = useTranslation(libraryNamespace);
  if (items.notSupported) {
    return (
      <p role="alert" className="text-text-muted">
        {t("items.filterNotSupported")}
      </p>
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-4">
      <p role="alert" className="text-text-muted">
        {t("items.loadFailed")}
      </p>
      <button type="button" onClick={items.retry} className={secondaryButtonClass}>
        {t("retry")}
      </button>
    </div>
  );
}

interface ItemListProps {
  readonly items: LibraryItems;
}

function ItemList({ items }: ItemListProps) {
  const { t } = useTranslation(libraryNamespace);
  const { focusItemId, markLoadMore } = useNewPageFocus(items);
  const loadMore = () => {
    markLoadMore();
    items.loadMore();
  };
  return (
    <>
      <ul aria-label={t("items.label")} className={itemGridClass}>
        {items.items.map((item) => (
          <li key={item.id}>
            <ItemCard item={item} focusOnMount={item.id === focusItemId} />
          </li>
        ))}
      </ul>
      {items.hasMore && (
        <div className="mt-8 flex flex-col items-center gap-3">
          {items.loadMoreFailed && !items.loadingMore && (
            <p role="alert" className="text-sm text-text-muted">
              {t("items.loadMoreFailed")}
            </p>
          )}
          {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
          <button
            type="button"
            onClick={loadMore}
            aria-disabled={items.loadingMore}
            aria-busy={items.loadingMore}
            className={secondaryButtonClass}
          >
            {t(loadMoreLabelKey(items))}
          </button>
        </div>
      )}
    </>
  );
}

function loadMoreLabelKey(items: LibraryItems): "items.loadingMore" | "retry" | "items.loadMore" {
  if (items.loadingMore) {
    return "items.loadingMore";
  }
  return items.loadMoreFailed ? "retry" : "items.loadMore";
}
