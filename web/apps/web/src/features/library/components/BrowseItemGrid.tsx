import type { LetterIndex } from "@mivo/api-client";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { CardFormat } from "../../../shared/lib/rowScroll";
import { EmptyState, LibraryIcon, Skeleton } from "../../../shared/ui";
import type { LibraryItems } from "../hooks/useLibraryItems";
import type { LibraryItemFilter } from "../lib/browseQuery";
import type { GridView } from "../lib/gridGeometry";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";
import type { BrowseCardProps } from "./browseCardProps";
import { VirtualItemGrid } from "./VirtualItemGrid";
import { WatchableItemCard } from "./WatchableItemCard";

export type { BrowseCardProps };

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
   * View style and card size of the device. Without it the grid keeps the default card size and
   * the cards their own sizes, which is what the features with their own cards use.
   */
  readonly view?: GridView | undefined;
  /** Shape of the cards, so episode stills get wider columns than posters. */
  readonly cardFormat?: CardFormat | undefined;
  /** Where each letter starts (title sort); the A-Z bar shows only with it. */
  readonly letters?: LetterIndex | undefined;
  /** Identifies the listing, so Back restores its scroll position; absent keeps none. */
  readonly scrollKey?: string | undefined;
}

function renderPosterCard({ item, size }: BrowseCardProps): ReactNode {
  return <WatchableItemCard item={item} size={size} />;
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
 * The listed items of a library as a virtual grid of cards, rows or details, page by page:
 * loading, error and empty states, skeletons for pages on their way and the A-Z bar. Features
 * with their own card (e.g. audiobooks) pass `renderCard` and their texts.
 */
export function BrowseItemGrid({
  items,
  filter,
  renderCard = renderPosterCard,
  texts,
  emptyIcon,
  view,
  cardFormat,
  letters,
  scrollKey,
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
    <VirtualItemGrid
      items={items}
      label={shown.label}
      loadMoreFailedText={shown.loadMoreFailed}
      loadingMoreText={shown.loadingMore}
      renderCard={renderCard}
      view={view}
      cardFormat={cardFormat}
      letters={letters}
      scrollKey={scrollKey}
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
