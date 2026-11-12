import type { ReactNode, Ref } from "react";

import { useSiblingLinks } from "../hooks/useSiblingLinks";
import type { ChildOrder } from "../lib/itemSearch";
import type { ItemView } from "../lib/itemView";
import { ChildrenList } from "./ChildrenList";
import { DetailLayout } from "./DetailLayout";
import { ItemExtras } from "./ItemExtras";
import { AncestorLinks, SiblingArrows } from "./ItemNavigation";

/** Props of the show and season pages. */
export interface ContainerPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
}

/** Props of {@link ShowPage}. */
export interface ShowPageProps extends ContainerPageProps {
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

/** A show: poster, title, summary, Play and its seasons, specials last. */
export function ShowPage({ headingRef, itemId, item, actions }: ShowPageProps) {
  return (
    <article>
      <DetailLayout headingRef={headingRef} item={item} aspect="poster" actions={actions} />
      <ChildrenList itemId={itemId} childKind="season" order="asc" />
      <ItemExtras item={item} />
    </article>
  );
}

/** Props of {@link SeasonPage}. */
export interface SeasonPageProps extends ContainerPageProps {
  readonly order: ChildOrder;
  /** Shown next to Play, e.g. "Mark as watched". */
  readonly actions?: ReactNode;
}

/** A season: link to the show, previous/next season, Play and its episodes in order. */
export function SeasonPage({ headingRef, itemId, item, order, actions }: SeasonPageProps) {
  const siblings = useSiblingLinks(item, "season");
  return (
    <article>
      <DetailLayout
        headingRef={headingRef}
        item={item}
        aspect="poster"
        actions={actions}
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
