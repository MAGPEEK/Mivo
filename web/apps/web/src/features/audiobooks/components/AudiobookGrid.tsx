import { useTranslation } from "react-i18next";

import {
  BrowseItemGrid,
  LibraryKindIcon,
  type BrowseCardProps,
  type BrowseItemTexts,
  type LibraryItemFilter,
  type LibraryItems,
} from "../../library";
import { audiobooksNamespace } from "../locales";
import { AudiobookCard } from "./AudiobookCard";

/** Props of {@link AudiobookGrid}. */
export interface AudiobookGridProps {
  /** The book pages the library's Browse listing loaded. */
  readonly books: LibraryItems;
  /** The toolbar filter, which picks the empty texts. */
  readonly filter: LibraryItemFilter;
}

function renderBookCard({ item, focusOnMount }: BrowseCardProps) {
  return <AudiobookCard book={item} focusOnMount={focusOnMount} />;
}

function useBookTexts(): BrowseItemTexts {
  const { t } = useTranslation(audiobooksNamespace);
  return {
    label: t("books.label"),
    loading: t("books.loading"),
    loadFailed: t("books.loadFailed"),
    loadingMore: t("books.loadingMore"),
    loadMoreFailed: t("books.loadMoreFailed"),
    emptyTitle: t("books.empty.title"),
    emptyHint: t("books.empty.hint"),
    emptyUnmatchedTitle: t("books.emptyUnmatched.title"),
    emptyUnmatchedHint: t("books.emptyUnmatched.hint"),
  };
}

/**
 * The books of an audiobook library as a grid of 2:3 covers with their authors, page by page;
 * the library's Browse grid supplies the states, "Load more" and the focus on the first new card.
 */
export function AudiobookGrid({ books, filter }: AudiobookGridProps) {
  const texts = useBookTexts();
  return (
    <BrowseItemGrid
      items={books}
      filter={filter}
      texts={texts}
      renderCard={renderBookCard}
      emptyIcon={<LibraryKindIcon kind="audiobooks" className="size-10" />}
    />
  );
}
