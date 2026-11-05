import type { ItemDetail } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { useFormatEpisodeCode } from "../../../shared/ui";
import { playerItemQueryOptions } from "../api/playerQueries";

/** What the player's title block shows of the item; everything is `undefined` until loaded. */
export interface PlayerItemInfo {
  /** Title of the item itself (the episode title for an episode). */
  readonly title: string | undefined;
  /** The item the title link opens: the show of an episode, otherwise the item. */
  readonly linkItemId: string;
  /** Text of the title link: the show's title for an episode, otherwise the item's title. */
  readonly linkTitle: string | undefined;
  /** Whether the item is an episode, which adds the `S1 · E3 · Title` line. */
  readonly episode: boolean;
  /** `S1 · E3` of an episode; `undefined` while a number is unknown. */
  readonly episodeCode: string | undefined;
  readonly year: number | undefined;
}

function showOf(item: ItemDetail) {
  return item.ancestors?.find((ancestor) => ancestor.kind === "show");
}

/**
 * The item the player plays, as the title block shows it (PL-02). The engine loads the same
 * query; this only reads its cache entry.
 */
export function usePlayerItem(itemId: string): PlayerItemInfo {
  const client = useApiClient();
  const formatEpisodeCode = useFormatEpisodeCode();
  const item = useQuery(playerItemQueryOptions(client, itemId)).data;
  if (item === undefined) {
    return {
      title: undefined,
      linkItemId: itemId,
      linkTitle: undefined,
      episode: false,
      episodeCode: undefined,
      year: undefined,
    };
  }
  const episode = item.kind === "episode";
  const show = episode ? showOf(item) : undefined;
  const season = item.ancestors?.find((ancestor) => ancestor.kind === "season");
  return {
    title: item.title,
    linkItemId: show?.id ?? itemId,
    linkTitle: show?.title ?? item.title,
    episode,
    episodeCode: episode
      ? formatEpisodeCode({
          season: season?.number,
          number: item.number,
          numberEnd: item.numberEnd,
        })
      : undefined,
    year: item.year,
  };
}
