import { Link } from "@tanstack/react-router";

import { MediaCard, Poster } from "../../../shared/ui";
import { LibraryKindIcon } from "../../library";
import { useArtistName } from "../hooks/useArtistName";
import type { AlbumCardModel } from "../lib/albumCard";

/** Props of {@link AlbumCard}. */
export interface AlbumCardProps {
  readonly album: AlbumCardModel;
}

// `sizes` of one column of the album grid (albumGridClass in MusicLibraryGrid): the same
// breakpoints and column counts as the video grid, so the viewport share is an upper bound.
const albumCoverSizes =
  "(min-width: 1280px) 12.5vw, (min-width: 1024px) 17vw, (min-width: 768px) 25vw, (min-width: 640px) 34vw, 50vw";

/** Card of one album in the grid: square cover, title, artist and year; opens the album. */
export function AlbumCard({ album }: AlbumCardProps) {
  const artistName = useArtistName(album.artistId);
  return (
    <MediaCard
      title={album.title}
      poster={
        <Poster
          imageRef={album.cover}
          aspect="square"
          alt={album.title}
          sizes={albumCoverSizes}
          icon={<LibraryKindIcon kind="music" className="size-1/4" />}
        />
      }
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: album.id }} {...linkProps} />
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
