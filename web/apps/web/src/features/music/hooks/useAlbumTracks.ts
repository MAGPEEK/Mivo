import { useInfiniteQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import { queueHandoffState } from "../../player";
import { albumTracksQueryOptions } from "../api/musicQueries";
import {
  groupByDisc,
  hasSeveralDiscs,
  playQueueIds,
  shuffleStartId,
  type DiscGroup,
} from "../lib/albumTracks";
import { usePagedItems, type PagedItems } from "./pagedItems";

/** The loaded tracks of an album grouped by disc, and the paging controls. */
export interface AlbumTracks extends PagedItems {
  readonly discs: readonly DiscGroup[];
  /** True when the album spans several discs, so each disc gets a heading. */
  readonly showDiscHeadings: boolean;
  /** The playable tracks' IDs in album order: the queue a click on a row hands to the player. */
  readonly queueIds: readonly string[];
}

/** Pages through the tracks of an album in disc and track order. */
export function useAlbumTracks(albumId: string): AlbumTracks {
  const client = useApiClient();
  const paged = usePagedItems(useInfiniteQuery(albumTracksQueryOptions(client, albumId)));
  const discs = useMemo(() => groupByDisc(paged.items), [paged.items]);
  const queueIds = useMemo(() => playQueueIds(paged.items), [paged.items]);
  return { ...paged, discs, showDiscHeadings: hasSeveralDiscs(discs), queueIds };
}

/** What the album's Play starts: its first playable track and the queue behind it. */
export type AlbumPlayTrack =
  | { readonly status: "pending" }
  | { readonly status: "error" }
  | {
      readonly status: "ready";
      readonly trackId: string | undefined;
      /** The playable tracks in album order; Play and Shuffle hand them to the player. */
      readonly queueIds: readonly string[];
    };

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
  return { status: "ready", trackId: tracks.queueIds[0], queueIds: tracks.queueIds };
}

/** The album's Shuffle: `shuffle` opens the player on a random track with shuffle on. */
export interface AlbumShuffle {
  /** False until the tracks are loaded and while none can be played. */
  readonly enabled: boolean;
  readonly shuffle: () => void;
}

/** Shuffle for an album, sharing the track list's query. */
export function useAlbumShuffle(albumId: string): AlbumShuffle {
  const router = useRouter();
  const play = useAlbumPlayTrack(albumId);
  const queueIds = play.status === "ready" ? play.queueIds : [];
  return {
    enabled: queueIds.length > 0,
    shuffle: () => {
      const start = shuffleStartId(queueIds, Math.random());
      if (start !== undefined) {
        void router.navigate({
          to: "/items/$itemId/play",
          params: { itemId: start },
          state: queueHandoffState({ ids: queueIds, shuffle: true }),
        });
      }
    },
  };
}
