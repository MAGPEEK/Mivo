import type { ReactNode, Ref } from "react";

import { AlbumShuffleButton, AlbumTrackList, ArtistAlbums } from "../../music";
import { useAlbumPlayView } from "../hooks/useAlbumPlayView";
import type { ItemView } from "../lib/itemView";
import { DetailLayout } from "./DetailLayout";
import { AncestorLinks } from "./ItemNavigation";

/** Props of {@link AlbumPage} and {@link ArtistPage}. */
export interface MusicPageProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

/**
 * An album: square cover, link to the artist, year and track count, Play and Shuffle for the
 * queue of its tracks, and the tracks grouped by disc.
 */
export function AlbumPage({ headingRef, itemId, item, actions }: MusicPageProps) {
  const play = useAlbumPlayView(itemId);
  return (
    <article>
      <DetailLayout
        headingRef={headingRef}
        item={{ ...item, play }}
        aspect="square"
        above={<AncestorLinks ancestors={item.ancestors} />}
        actions={
          <>
            <AlbumShuffleButton albumId={itemId} />
            {actions}
          </>
        }
      />
      <AlbumTrackList albumId={itemId} />
    </article>
  );
}

/**
 * An artist: picture and name, then the artist's albums oldest first. Playing a whole artist
 * needs the tracks of every album and waits for a server-side listing (BACKLOG, M5-71), so
 * the page has no Play button.
 */
export function ArtistPage({ headingRef, itemId, item, actions }: MusicPageProps) {
  return (
    <article>
      <DetailLayout
        headingRef={headingRef}
        item={{ ...item, play: { kind: "none" } }}
        aspect="square"
        actions={actions}
      />
      <ArtistAlbums artistId={itemId} />
    </article>
  );
}
