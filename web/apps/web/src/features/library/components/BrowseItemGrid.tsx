import type { ItemSummary } from "@mivo/api-client";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { EmptyState, LibraryIcon, Skeleton } from "../../../shared/ui";
import type { LibraryItems } from "../hooks/useLibraryItems";
import { useNewPageFocus } from "../hooks/useNewPageFocus";
import type { LibraryItemFilter } from "../lib/browseQuery";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";
import { itemGridClass } from "./gridLayout";
import { ItemCard } from "./ItemCard";

/** The translated texts of a {@link BrowseItemGrid}; the defaults speak of videos. */
export interface BrowseItemTexts {
  /** Accessible name of the list. */
  readonly label: string;
  readonly loading: string;
  readonly loadFailed: string;
  readonly loadingMore: string;
  readonly loadMoreFailed: string;
  readonly emptyTitle: string;
  readonly emptyHint: string;
  /** Shown instead of the empty texts when the Unmatched filter finds nothing. */
  readonly emptyUnmatchedTitle: string;
  readonly emptyUnmatchedHint: string;
}

/** What a {@link BrowseItemGrid} hands to its card renderer. */
export interface BrowseCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it mounts: the first card of a page "Load more" loaded. */
  readonly focusOnMount: boolean;
}

/** Props of {@link BrowseItemGrid}. */
export interface BrowseItemGridProps {
  readonly items: LibraryItems;
  /** The toolbar filter, which picks the empty texts. */
  readonly filter: LibraryItemFilter;
  /** Renders one card; defaults to the poster card of a video. */
  readonly renderCard?: ((card: BrowseCardProps) => ReactNode) | undefined;
  /** Texts for another kind of item; defaults to the video texts. */
  readonly texts?: BrowseItemTexts | undefined;
  /** Icon of the empty state; defaults to the generic library icon. */
  readonly emptyIcon?: ReactNode;
}

function renderPosterCard({ item, focusOnMount }: BrowseCardProps): ReactNode {
  return <ItemCard item={item} focusOnMount={focusOnMount} />;
}

function useVideoTexts(): BrowseItemTexts {
  const { t } = useTranslation(libraryNamespace);
  return {
    label: t("items.label"),
    loading: t("items.loading"),
    loadFailed: t("items.loadFailed"),
    loadingMore: t("items.loadingMore"),
    loadMoreFailed: t("items.loadMoreFailed"),
    emptyTitle: t("items.empty.title"),
    emptyHint: t("items.empty.hint"),
    emptyUnmatchedTitle: t("items.emptyUnmatched.title"),
    emptyUnmatchedHint: t("items.emptyUnmatched.hint"),
  };
}

/**
 * The listed items of a library as a dense grid of 2:3 cards, page by page: loading, error and
 * empty states, "Load more" and the focus move to the first new card. Features with their own
 * card (e.g. audiobooks) pass `renderCard` and their texts.
 */
export function BrowseItemGrid({
  items,
  filter,
  renderCard = renderPosterCard,
  texts,
  emptyIcon,
}: BrowseItemGridProps) {
  const videoTexts = useVideoTexts();
  const shown = texts ?? videoTexts;
  if (items.status === "pending") {
    return (
      <div role="status" className="w-40">
        <Skeleton label={shown.loading} className="aspect-2/3 w-full rounded-md" />
      </div>
    );
  }
  if (items.status === "error") {
    return <ItemsLoadError items={items} texts={shown} />;
  }
  if (items.items.length === 0) {
    const unmatched = filter === "unmatched";
    return (
      <EmptyState
        icon={emptyIcon ?? <LibraryIcon className="size-10" />}
        title={unmatched ? shown.emptyUnmatchedTitle : shown.emptyTitle}
        description={unmatched ? shown.emptyUnmatchedHint : shown.emptyHint}
      />
    );
  }
  return <ItemList items={items} texts={shown} renderCard={renderCard} />;
}

interface ItemsLoadErrorProps {
  readonly items: LibraryItems;
  readonly texts: BrowseItemTexts;
}

function ItemsLoadError({ items, texts }: ItemsLoadErrorProps) {
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
        {texts.loadFailed}
      </p>
      <button type="button" onClick={items.retry} className={secondaryButtonClass}>
        {t("retry")}
      </button>
    </div>
  );
}

interface ItemListProps {
  readonly items: LibraryItems;
  readonly texts: BrowseItemTexts;
  readonly renderCard: (card: BrowseCardProps) => ReactNode;
}

function ItemList({ items, texts, renderCard }: ItemListProps) {
  const { t } = useTranslation(libraryNamespace);
  const { focusItemId, markLoadMore } = useNewPageFocus(items);
  const loadMore = () => {
    markLoadMore();
    items.loadMore();
  };
  return (
    <>
      <ul aria-label={texts.label} className={itemGridClass}>
        {items.items.map((item) => (
          <li key={item.id}>{renderCard({ item, focusOnMount: item.id === focusItemId })}</li>
        ))}
      </ul>
      {items.hasMore && (
        <div className="mt-8 flex flex-col items-center gap-3">
          {items.loadMoreFailed && !items.loadingMore && (
            <p role="alert" className="text-sm text-text-muted">
              {texts.loadMoreFailed}
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
            {loadMoreLabel(items, texts, t("items.loadMore"), t("retry"))}
          </button>
        </div>
      )}
    </>
  );
}

function loadMoreLabel(
  items: LibraryItems,
  texts: BrowseItemTexts,
  loadMore: string,
  retry: string,
): string {
  if (items.loadingMore) {
    return texts.loadingMore;
  }
  return items.loadMoreFailed ? retry : loadMore;
}
