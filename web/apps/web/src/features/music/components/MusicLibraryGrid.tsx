import { useTranslation } from "react-i18next";

import {
  BrowseItemGrid,
  LibraryKindIcon,
  type BrowseCardProps,
  type BrowseItemTexts,
  type LibraryItemFilter,
  type LibraryItems,
} from "../../library";
import { toAlbumCardModel } from "../lib/albumCard";
import { musicNamespace } from "../locales";
import { AlbumCard } from "./AlbumCard";

/** Props of {@link MusicLibraryGrid}. */
export interface MusicLibraryGridProps {
  /** The album pages the library's Browse listing loaded. */
  readonly albums: LibraryItems;
  /** The toolbar filter, which picks the empty texts. */
  readonly filter: LibraryItemFilter;
}

function renderAlbumCard({ item, focusOnMount }: BrowseCardProps) {
  return <AlbumCard album={toAlbumCardModel(item)} focusOnMount={focusOnMount} />;
}

function useAlbumTexts(): BrowseItemTexts {
  const { t } = useTranslation(musicNamespace);
  return {
    label: t("albums.label"),
    loading: t("albums.loading"),
    loadFailed: t("albums.loadFailed"),
    loadingMore: t("albums.loadingMore"),
    loadMoreFailed: t("albums.loadMoreFailed"),
    emptyTitle: t("albums.empty.title"),
    emptyHint: t("albums.empty.hint"),
    emptyUnmatchedTitle: t("albums.emptyUnmatched.title"),
    emptyUnmatchedHint: t("albums.emptyUnmatched.hint"),
  };
}

/**
 * The albums of a music library as a grid of square covers, page by page; the library's
 * Browse grid supplies the states, "Load more" and the focus on the first new card.
 */
export function MusicLibraryGrid({ albums, filter }: MusicLibraryGridProps) {
  const texts = useAlbumTexts();
  return (
    <BrowseItemGrid
      items={albums}
      filter={filter}
      texts={texts}
      renderCard={renderAlbumCard}
      emptyIcon={<LibraryKindIcon kind="music" className="size-10" />}
      cardFormat="square"
    />
  );
}
