import type { ReactNode, Ref } from "react";

import { AlbumTrackList } from "../../music";
import { useAlbumPlayView } from "../hooks/useAlbumPlayView";
import type { ItemView } from "../lib/itemView";
import { DetailLayout } from "./DetailLayout";
import { AncestorLinks } from "./ItemNavigation";

/** Props of {@link AlbumPage}. */
export interface AlbumPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

/**
 * An album: square cover, link to the artist, year and track count, Play for the first track
 * and the tracks grouped by disc.
 */
export function AlbumPage({ headingRef, itemId, item, actions }: AlbumPageProps) {
  const play = useAlbumPlayView(itemId);
  return (
    <article>
      <DetailLayout
        headingRef={headingRef}
        item={{ ...item, play }}
        aspect="square"
        above={<AncestorLinks ancestors={item.ancestors} />}
        actions={actions}
      />
      <AlbumTrackList albumId={itemId} />
    </article>
  );
}
