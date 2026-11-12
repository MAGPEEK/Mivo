import type { ItemSummary } from "@mivo/api-client";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { CardFormat } from "../../../shared/lib/rowScroll";
import { EmptyState, LibraryIcon, Skeleton } from "../../../shared/ui";
import type { LibraryItems } from "../hooks/useLibraryItems";
import { useNewPageFocus } from "../hooks/useNewPageFocus";
import type { ViewPreference } from "../hooks/useViewPreference";
import type { LibraryItemFilter } from "../lib/browseQuery";
import { cardSize, type CardSize } from "../lib/viewStyle";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";
import { DetailsView, ListView } from "./ItemRows";
import { itemGridClass } from "./gridLayout";
import { WatchableItemCard } from "./WatchableItemCard";

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
  /** The card size of the size slider; `undefined` keeps the card's own fixed sizes. */
  readonly size?: CardSize | undefined;
}

/** Props of {@link BrowseItemGrid}. */
export interface BrowseItemGridProps {
  readonly items: LibraryItems;
  /** What the filters narrow, which picks the empty texts. */
  readonly filter: LibraryItemFilter;
  /** Renders one card; defaults to the poster card of a video. */
  readonly renderCard?: ((card: BrowseCardProps) => ReactNode) | undefined;
  /** Texts for another kind of item; defaults to the video texts. */
  readonly texts?: BrowseItemTexts | undefined;
  /** Icon of the empty state; defaults to the generic library icon. */
  readonly emptyIcon?: ReactNode;
  /**
   * View style and card size of the device. Without it the grid keeps its fixed breakpoint
   * columns, which is what the features with their own cards use.
   */
  readonly view?: Pick<ViewPreference, "viewStyle" | "cardStep"> | undefined;
  /** Shape of the cards, so episode stills get wider columns than posters. */
  readonly cardFormat?: CardFormat | undefined;
}

function renderPosterCard({ item, focusOnMount, size }: BrowseCardProps): ReactNode {
  return <WatchableItemCard item={item} focusOnMount={focusOnMount} size={size} />;
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
  view,
  cardFormat,
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
    return <ItemsEmpty filter={filter} texts={shown} icon={emptyIcon} />;
  }
  return (
    <ItemList
      items={items}
      texts={shown}
      renderCard={renderCard}
      view={view}
      cardFormat={cardFormat}
    />
  );
}

interface ItemsEmptyProps {
  readonly filter: LibraryItemFilter;
  readonly texts: BrowseItemTexts;
  readonly icon: ReactNode;
}

function ItemsEmpty({ filter, texts, icon }: ItemsEmptyProps) {
  const { t } = useTranslation(libraryNamespace);
  // Any filter but Unmatched speaks of the filters, whatever kind of item is listed.
  const shown = {
    all: { title: texts.emptyTitle, hint: texts.emptyHint },
    unmatched: { title: texts.emptyUnmatchedTitle, hint: texts.emptyUnmatchedHint },
    filtered: { title: t("items.emptyFiltered.title"), hint: t("items.emptyFiltered.hint") },
  }[filter];
  return (
    <EmptyState
      icon={icon ?? <LibraryIcon className="size-10" />}
      title={shown.title}
      description={shown.hint}
    />
  );
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
  readonly view: BrowseItemGridProps["view"];
  readonly cardFormat: CardFormat | undefined;
}

function ItemList({ items, texts, renderCard, view, cardFormat }: ItemListProps) {
  const { t } = useTranslation(libraryNamespace);
  const { focusItemId, markLoadMore } = useNewPageFocus(items);
  const loadMore = () => {
    markLoadMore();
    items.loadMore();
  };
  return (
    <>
      <ItemsBody
        items={items.items}
        label={texts.label}
        focusItemId={focusItemId}
        renderCard={renderCard}
        view={view}
        cardFormat={cardFormat}
      />
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

interface ItemsBodyProps {
  readonly items: readonly ItemSummary[];
  readonly label: string;
  readonly focusItemId: string | undefined;
  readonly renderCard: (card: BrowseCardProps) => ReactNode;
  readonly view: BrowseItemGridProps["view"];
  readonly cardFormat: CardFormat | undefined;
}

// The list views take the items only, so they also serve season pages (M5-44).
function ItemsBody({ items, label, focusItemId, renderCard, view, cardFormat }: ItemsBodyProps) {
  if (view?.viewStyle === "list") {
    return <ListView items={items} label={label} focusItemId={focusItemId} />;
  }
  if (view?.viewStyle === "details") {
    return <DetailsView items={items} label={label} focusItemId={focusItemId} />;
  }
  const size = view === undefined ? undefined : cardSize(view.cardStep, cardFormat);
  return (
    <ul
      aria-label={label}
      className={size === undefined ? itemGridClass : "grid gap-x-4 gap-y-6"}
      style={
        size === undefined
          ? undefined
          : { gridTemplateColumns: `repeat(auto-fill, minmax(${String(size.columnWidth)}px, 1fr))` }
      }
    >
      {items.map((item) => (
        <li key={item.id}>{renderCard({ item, focusOnMount: item.id === focusItemId, size })}</li>
      ))}
    </ul>
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
