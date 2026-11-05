import { useInfiniteQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { useFormatEpisodeCode } from "../../../shared/ui";
import { childrenQueryOptions } from "../api/childrenQuery";
import type { ItemView } from "../lib/itemView";

/**
 * The `S1 · E3` label of an episode, or `undefined` while its numbers are unknown.
 *
 * The number is read from the season's children, the list the season page loads anyway and
 * the query cache shares; this predates API 1.3.0, which added `number` to `ItemDetail`. An
 * episode beyond the first page of a large season therefore gets no label yet; reading the
 * detail instead is in docs/tasks/BACKLOG.md (m5-24 review nits).
 */
export function useEpisodeCode(episodeId: string, item: ItemView): string | undefined {
  const client = useApiClient();
  const formatEpisodeCode = useFormatEpisodeCode();
  const season = item.ancestors.find((ancestor) => ancestor.kind === "season");
  const query = useInfiniteQuery({
    ...childrenQueryOptions(client, season?.id ?? "", "asc"),
    enabled: season !== undefined,
  });
  const summary = query.data?.pages
    .flatMap((page) => page.items)
    .find((child) => child.id === episodeId);
  return formatEpisodeCode({
    season: season?.number,
    number: summary?.number,
    numberEnd: summary?.numberEnd,
  });
}
