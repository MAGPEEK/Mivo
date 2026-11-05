import { useTranslation } from "react-i18next";

import {
  BrowseItemGrid,
  LibraryKindIcon,
  type BrowseCardProps,
  type BrowseItemTexts,
  type LibraryItemFilter,
  type LibraryItems,
} from "../../library";
import { useAlbumContents } from "../hooks/useAlbumContents";
import { photosNamespace } from "../locales";
import { PhotoAlbumCard } from "./PhotoAlbumCard";
import { PhotoTile } from "./PhotoTile";

// A listing of a photos library holds albums, photos or (inside an album) both: each kind gets
// its own card.
function renderPhotoCard({ item, focusOnMount }: BrowseCardProps) {
  return item.kind === "photo_album" ? (
    <PhotoAlbumCard album={item} focusOnMount={focusOnMount} />
  ) : (
    <PhotoTile photo={item} focusOnMount={focusOnMount} />
  );
}

type TextGroup = "albums" | "photos" | "contents";

function useGridTexts(group: TextGroup): BrowseItemTexts {
  const { t } = useTranslation(photosNamespace);
  // An album's contents take no filter, so its empty texts serve the Unmatched case too.
  const unmatched = group === "contents" ? "contents.empty" : (`${group}.emptyUnmatched` as const);
  return {
    label: t(`${group}.label`),
    loading: t(`${group}.loading`),
    loadFailed: t(`${group}.loadFailed`),
    loadingMore: t(`${group}.loadingMore`),
    loadMoreFailed: t(`${group}.loadMoreFailed`),
    emptyTitle: t(`${group}.empty.title`),
    emptyHint: t(`${group}.empty.hint`),
    emptyUnmatchedTitle: t(`${unmatched}.title`),
    emptyUnmatchedHint: t(`${unmatched}.hint`),
  };
}

const emptyIcon = <LibraryKindIcon kind="photos" className="size-10" />;

/** Props of the photo library grids {@link PhotoAlbumGrid} and {@link PhotoGrid}. */
export interface PhotoLibraryGridProps {
  /** The pages the library's Browse listing loaded. */
  readonly items: LibraryItems;
  /** The toolbar filter, which picks the empty texts. */
  readonly filter: LibraryItemFilter;
}

/**
 * The top-level albums of a photos library (Browse type "photo_album") as square covers, page
 * by page; loose photos in the library root appear as tiles among them.
 */
export function PhotoAlbumGrid({ items, filter }: PhotoLibraryGridProps) {
  return (
    <BrowseItemGrid
      items={items}
      filter={filter}
      texts={useGridTexts("albums")}
      renderCard={renderPhotoCard}
      emptyIcon={emptyIcon}
    />
  );
}

/** Every photo of a photos library (Browse type "photo") as square tiles, page by page. */
export function PhotoGrid({ items, filter }: PhotoLibraryGridProps) {
  return (
    <BrowseItemGrid
      items={items}
      filter={filter}
      texts={useGridTexts("photos")}
      renderCard={renderPhotoCard}
      emptyIcon={emptyIcon}
    />
  );
}

/** Props of {@link PhotoAlbumContents}. */
export interface PhotoAlbumContentsProps {
  readonly albumId: string;
}

/**
 * The contents of a photo album, page by page with the server's cursor: its sub-albums as
 * covers first, then its photos as tiles by capture time.
 */
export function PhotoAlbumContents({ albumId }: PhotoAlbumContentsProps) {
  const contents = useAlbumContents(albumId);
  return (
    <BrowseItemGrid
      items={contents}
      filter="all"
      texts={useGridTexts("contents")}
      renderCard={renderPhotoCard}
      emptyIcon={emptyIcon}
    />
  );
}
