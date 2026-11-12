import { useInfiniteQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import { artistAlbumsQueryOptions } from "../api/musicQueries";
import { toAlbumCardModel, type AlbumCardModel } from "../lib/albumCard";
import { sortAlbumsByYear } from "../lib/artistAlbums";
import { usePagedItems, type PagedItems } from "./pagedItems";

/** The loaded albums of an artist as cards, oldest first, and the paging controls. */
export interface ArtistAlbums extends PagedItems {
  readonly albums: readonly AlbumCardModel[];
}

/** Pages through the albums of an artist. */
export function useArtistAlbums(artistId: string): ArtistAlbums {
  const client = useApiClient();
  const paged = usePagedItems(useInfiniteQuery(artistAlbumsQueryOptions(client, artistId)));
  const albums = useMemo(() => sortAlbumsByYear(paged.items).map(toAlbumCardModel), [paged.items]);
  return { ...paged, albums };
}
