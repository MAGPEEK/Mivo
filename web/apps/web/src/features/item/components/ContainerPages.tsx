import type { Ref } from "react";

import { useSiblingLinks } from "../hooks/useSiblingLinks";
import type { ChildOrder } from "../lib/itemSearch";
import type { ItemView } from "../lib/itemView";
import { ChildrenList } from "./ChildrenList";
import { ItemHeader } from "./ItemHeader";
import { AncestorLinks, SiblingArrows } from "./ItemNavigation";

/** Props of the show and season pages. */
export interface ContainerPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
}

/** A show: poster, title, summary, Play and its seasons, specials last. */
export function ShowPage({ headingRef, itemId, item }: ContainerPageProps) {
  return (
    <article>
      <ItemHeader headingRef={headingRef} item={item} aspect="poster" />
      <ChildrenList itemId={itemId} childKind="season" order="asc" />
    </article>
  );
}

/** Props of {@link SeasonPage}. */
export interface SeasonPageProps extends ContainerPageProps {
  readonly order: ChildOrder;
}

/** A season: link to the show, previous/next season, Play and its episodes in order. */
export function SeasonPage({ headingRef, itemId, item, order }: SeasonPageProps) {
  const siblings = useSiblingLinks(item, "season");
  return (
    <article>
      <ItemHeader
        headingRef={headingRef}
        item={item}
        aspect="poster"
        above={
          <div className="flex flex-wrap items-center gap-4">
            <AncestorLinks ancestors={item.ancestors} />
            <SiblingArrows links={siblings} />
          </div>
        }
      />
      <ChildrenList itemId={itemId} childKind="episode" order={order} />
    </article>
  );
}
