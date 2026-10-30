import { useTranslation } from "react-i18next";

import { EmptyState, LibraryIcon, Skeleton } from "../../../shared/ui";
import { useLibraryItems, type LibraryItems } from "../hooks/useLibraryItems";
import { useNewPageFocus } from "../hooks/useNewPageFocus";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";
import { ItemCard } from "./ItemCard";

/** Props of {@link LibraryItemGrid}. */
export interface LibraryItemGridProps {
  readonly libraryId: string;
}

// Dense poster rows like the rest of the design: more columns as the window widens.
const gridClass =
  "grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8";

/** The items of a library as a poster grid, page by page. */
export function LibraryItemGrid({ libraryId }: LibraryItemGridProps) {
  const { t } = useTranslation(libraryNamespace);
  const items = useLibraryItems(libraryId);
  if (items.status === "pending") {
    return (
      <div role="status" className="w-40">
        <Skeleton label={t("items.loading")} className="aspect-2/3 w-full rounded-md" />
      </div>
    );
  }
  if (items.status === "error") {
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
  if (items.items.length === 0) {
    return (
      <EmptyState
        icon={<LibraryIcon className="size-10" />}
        title={t("items.empty.title")}
        description={t("items.empty.hint")}
      />
    );
  }
  return <ItemList items={items} />;
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
      <ul aria-label={t("items.label")} className={gridClass}>
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
