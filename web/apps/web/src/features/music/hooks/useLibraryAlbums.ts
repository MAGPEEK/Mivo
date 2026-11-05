import { useMemo } from "react";

import { toAlbumCardModel, type AlbumCardModel } from "../lib/albumCard";
import type { PagedItems } from "./pagedItems";

/** The loaded albums of a music library as cards, and the paging controls. */
export interface LibraryAlbums extends PagedItems {
  readonly albums: readonly AlbumCardModel[];
}

/**
 * The album cards of the pages the library's Browse listing loaded (type "album"); the
 * listing owns the query, so sort, filter and paging stay those of the toolbar.
 */
export function useLibraryAlbums(pages: PagedItems): LibraryAlbums {
  const albums = useMemo(() => pages.items.map(toAlbumCardModel), [pages.items]);
  return { ...pages, albums };
}
