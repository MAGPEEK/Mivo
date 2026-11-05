import type { Ref } from "react";
import { useTranslation } from "react-i18next";

import { useEpisodeCode } from "../hooks/useEpisodeCode";
import { useSiblingLinks } from "../hooks/useSiblingLinks";
import type { ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";
import { ItemDetails } from "./ItemDetails";
import { AncestorLinks, SiblingArrows } from "./ItemNavigation";

/** Props of {@link EpisodePage}. */
export interface EpisodePageProps {
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly item: ItemView;
}

/**
 * An episode: links to its show and season, previous/next episode, then the technical details
 * and Play of every playable item.
 */
export function EpisodePage({ headingRef, item }: EpisodePageProps) {
  const { t } = useTranslation(itemNamespace);
  const siblings = useSiblingLinks(item, "episode");
  const code = useEpisodeCode(item);
  const leadFacts = [
    ...(code === undefined ? [] : [code]),
    ...(item.airDate === undefined ? [] : [t("episode.airDate", { date: item.airDate })]),
  ];
  return (
    <ItemDetails
      headingRef={headingRef}
      item={item}
      leadFacts={leadFacts}
      above={
        <div className="flex flex-wrap items-center gap-4">
          <AncestorLinks ancestors={item.ancestors} />
          <SiblingArrows links={siblings} />
        </div>
      }
    />
  );
}
