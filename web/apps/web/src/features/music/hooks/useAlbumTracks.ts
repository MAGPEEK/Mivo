import { useInfiniteQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import { albumTracksQueryOptions } from "../api/musicQueries";
import {
  firstPlayableTrackId,
  groupByDisc,
  hasSeveralDiscs,
  type DiscGroup,
} from "../lib/albumTracks";
import { usePagedItems, type PagedItems } from "./pagedItems";

/** The loaded tracks of an album grouped by disc, and the paging controls. */
export interface AlbumTracks extends PagedItems {
  readonly discs: readonly DiscGroup[];
  /** True when the album spans several discs, so each disc gets a heading. */
  readonly showDiscHeadings: boolean;
}

/** Pages through the tracks of an album in disc and track order. */
export function useAlbumTracks(albumId: string): AlbumTracks {
  const client = useApiClient();
  const paged = usePagedItems(useInfiniteQuery(albumTracksQueryOptions(client, albumId)));
  const discs = useMemo(() => groupByDisc(paged.items), [paged.items]);
  return { ...paged, discs, showDiscHeadings: hasSeveralDiscs(discs) };
}

/** What the album's Play starts: its first playable track, once the tracks are loaded. */
export type AlbumPlayTrack =
  | { readonly status: "pending" }
  | { readonly status: "error" }
  | { readonly status: "ready"; readonly trackId: string | undefined };

/**
 * The track the album's Play button starts. It shares the track list's query, so it costs no
 * extra request; `error` when the tracks could not be loaded (the list offers the retry).
 */
export function useAlbumPlayTrack(albumId: string): AlbumPlayTrack {
  const tracks = useAlbumTracks(albumId);
  if (tracks.status === "pending") {
    return { status: "pending" };
  }
  if (tracks.status === "error") {
    return { status: "error" };
  }
  return { status: "ready", trackId: firstPlayableTrackId(tracks.items) };
}
