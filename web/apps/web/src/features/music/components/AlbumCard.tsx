import { Link } from "@tanstack/react-router";

import { MediaCard, Poster } from "../../../shared/ui";
import { itemPosterSizes, LibraryKindIcon, useFocusOnMount } from "../../library";
import { useArtistName } from "../hooks/useArtistName";
import type { AlbumCardModel } from "../lib/albumCard";

/** Props of {@link AlbumCard}. */
export interface AlbumCardProps {
  readonly album: AlbumCardModel;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

/** Card of one album in the grid: square cover, title, artist and year; opens the album. */
export function AlbumCard({ album, focusOnMount = false }: AlbumCardProps) {
  const artistName = useArtistName(album.artistId);
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  return (
    <MediaCard
      title={album.title}
      poster={
        <Poster
          imageRef={album.cover}
          aspect="square"
          alt={album.title}
          // One column of the library grid, as wide as a poster column.
          sizes={itemPosterSizes}
          icon={<LibraryKindIcon kind="music" className="size-1/4" />}
        />
      }
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: album.id }} ref={linkRef} {...linkProps} />
      )}
      details={
        <>
          {artistName !== undefined && <span className="truncate">{artistName}</span>}
          {album.year !== "" && <span>{album.year}</span>}
        </>
      }
    />
  );
}
