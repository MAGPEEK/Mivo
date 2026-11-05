import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { Poster } from "../../../shared/ui";
import { itemPosterSizes, LibraryKindIcon, useFocusOnMount } from "../../library";
import { usePhotoTile } from "../hooks/usePhotoTile";
import { UnsupportedPhoto } from "./UnsupportedPhoto";

/** Props of {@link PhotoTile}. */
export interface PhotoTileProps {
  readonly photo: ItemSummary;
  /** Focus the tile when it appears, e.g. the first tile of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

const tileClass =
  "group relative block overflow-hidden rounded-md bg-surface-raised focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * One photo in a grid: a square tile cropped from the photo that opens it. The image loads
 * lazily; a photo the server cannot render says so instead, a video shows its length.
 */
export function PhotoTile({ photo, focusOnMount = false }: PhotoTileProps) {
  const tile = usePhotoTile(photo);
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: tile.id }}
      ref={linkRef}
      aria-label={tile.label}
      className={tileClass}
    >
      {tile.unsupported ? (
        <UnsupportedPhoto />
      ) : (
        <Poster
          imageRef={tile.image}
          aspect="square"
          alt=""
          sizes={itemPosterSizes}
          icon={<LibraryKindIcon kind="photos" className="size-1/4" />}
          className="transition-opacity group-hover:opacity-90"
        />
      )}
      {tile.durationText !== undefined && (
        <span
          aria-hidden="true"
          className="absolute right-1.5 bottom-1.5 rounded-sm bg-background/80 px-1.5 py-0.5 text-xs font-medium text-text"
        >
          {tile.durationText}
        </span>
      )}
    </Link>
  );
}
