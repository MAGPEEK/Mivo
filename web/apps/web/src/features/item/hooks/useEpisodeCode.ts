import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { childrenQueryOptions } from "../api/childrenQuery";
import { episodeRange, isMultiEpisode } from "../lib/episodeLabel";
import type { ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/**
 * The `S1 · E3` label of an episode, or `undefined` while its numbers are unknown.
 *
 * The item detail carries no episode number (API 1.2.0), so the number is read from the
 * season's children, the list the season page loads anyway and the query cache shares. An
 * episode beyond the first page of a large season gets no label until the contract adds the
 * numbers to `ItemDetail`.
 */
export function useEpisodeCode(episodeId: string, item: ItemView): string | undefined {
  const client = useApiClient();
  const { t } = useTranslation(itemNamespace);
  const season = item.ancestors.find((ancestor) => ancestor.kind === "season");
  const query = useInfiniteQuery({
    ...childrenQueryOptions(client, season?.id ?? "", "asc"),
    enabled: season !== undefined,
  });
  const summary = query.data?.pages
    .flatMap((page) => page.items)
    .find((child) => child.id === episodeId);
  const range = episodeRange(summary?.number, summary?.numberEnd);
  if (season?.number === undefined || range === undefined) {
    return undefined;
  }
  return isMultiEpisode(range)
    ? t("episode.codeRange", { season: season.number, first: range.first, last: range.last })
    : t("episode.code", { season: season.number, number: range.first });
}
