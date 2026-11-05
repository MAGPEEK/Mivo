import { useFormatEpisodeCode } from "../../../shared/ui";
import type { ItemView } from "../lib/itemView";

/**
 * The `S1 · E3` label of an episode, or `undefined` while its numbers are unknown. The episode
 * number comes from the episode's own detail (API 1.3.0), the season number from its season
 * ancestor, so the label also works beyond the first page of a large season.
 */
export function useEpisodeCode(item: ItemView): string | undefined {
  const formatEpisodeCode = useFormatEpisodeCode();
  const season = item.ancestors.find((ancestor) => ancestor.kind === "season");
  return formatEpisodeCode({
    season: season?.number,
    number: item.number,
    numberEnd: item.numberEnd,
  });
}
