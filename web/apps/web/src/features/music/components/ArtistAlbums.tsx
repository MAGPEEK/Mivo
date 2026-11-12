import { useId } from "react";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass, Skeleton } from "../../../shared/ui";
import { itemGridClass } from "../../library";
import { useArtistAlbums, type ArtistAlbums as ArtistAlbumsState } from "../hooks/useArtistAlbums";
import { musicNamespace } from "../locales";
import { AlbumCard } from "./AlbumCard";
import { LoadMoreButton } from "./LoadMoreButton";

/** Props of {@link ArtistAlbums}. */
export interface ArtistAlbumsProps {
  readonly artistId: string;
}

/** The albums of an artist as square covers, oldest first. */
export function ArtistAlbums({ artistId }: ArtistAlbumsProps) {
  const { t } = useTranslation(musicNamespace);
  const headingId = useId();
  const albums = useArtistAlbums(artistId);
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="mb-4 text-lg font-semibold">
        {t("artist.albums")}
      </h2>
      <AlbumsBody albums={albums} />
    </section>
  );
}

function AlbumsBody({ albums }: { readonly albums: ArtistAlbumsState }) {
  const { t } = useTranslation(musicNamespace);
  if (albums.status === "pending") {
    return (
      <div role="status" className="w-60">
        <Skeleton label={t("artist.loading")} className="h-8 w-full rounded-md" />
      </div>
    );
  }
  if (albums.status === "error") {
    return (
      <div className="flex flex-wrap items-center gap-4">
        <p role="alert" className="text-text-muted">
          {t("artist.loadFailed")}
        </p>
        <button type="button" onClick={albums.retry} className={secondaryButtonClass}>
          {t("artist.retry")}
        </button>
      </div>
    );
  }
  if (albums.albums.length === 0) {
    return <p className="text-text-muted">{t("artist.empty")}</p>;
  }
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y/no-redundant-roles -- list-style: none drops the list semantics in Safari */}
      <ul role="list" aria-label={t("artist.albums")} className={itemGridClass}>
        {albums.albums.map((album) => (
          <li key={album.id}>
            <AlbumCard album={album} />
          </li>
        ))}
      </ul>
      {albums.hasMore && (
        <LoadMoreButton
          paged={albums}
          texts={{
            loadMore: t("artist.loadMore"),
            loadingMore: t("artist.loadingMore"),
            failed: t("artist.loadMoreFailed"),
            retry: t("artist.retry"),
          }}
        />
      )}
    </>
  );
}
