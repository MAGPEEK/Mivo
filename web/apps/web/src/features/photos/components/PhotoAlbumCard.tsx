import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { MediaCard, Poster } from "../../../shared/ui";
import { itemPosterSizes, LibraryKindIcon, useFocusOnMount } from "../../library";
import { photosNamespace } from "../locales";

/** Props of {@link PhotoAlbumCard}. */
export interface PhotoAlbumCardProps {
  readonly album: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

/**
 * Card of one photo album: its newest photo as a square cover, the name and the number of
 * photos in it and its sub-albums; opens the album.
 */
export function PhotoAlbumCard({ album, focusOnMount = false }: PhotoAlbumCardProps) {
  const { t } = useTranslation(photosNamespace);
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  return (
    <MediaCard
      title={album.title}
      poster={
        <Poster
          imageRef={album.square}
          aspect="square"
          alt={album.title}
          sizes={itemPosterSizes}
          icon={<LibraryKindIcon kind="photos" className="size-1/4" />}
        />
      }
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: album.id }} ref={linkRef} {...linkProps} />
      )}
      details={
        album.leafCount !== undefined && (
          <span>{t("album.photoCount", { count: album.leafCount })}</span>
        )
      }
    />
  );
}
