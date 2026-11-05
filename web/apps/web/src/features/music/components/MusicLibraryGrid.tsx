import { useTranslation } from "react-i18next";

import { EmptyState, secondaryButtonClass, Skeleton } from "../../../shared/ui";
import { LibraryKindIcon } from "../../library";
import type { PagedItems } from "../hooks/pagedItems";
import { useLibraryAlbums, type LibraryAlbums } from "../hooks/useLibraryAlbums";
import { musicNamespace } from "../locales";
import { AlbumCard } from "./AlbumCard";
import { LoadMoreButton } from "./LoadMoreButton";

/** Props of {@link MusicLibraryGrid}. */
export interface MusicLibraryGridProps {
  /** The album pages the library's Browse listing loaded. */
  readonly albums: PagedItems;
}

// The same dense grid as the video grid; square covers instead of 2:3 posters.
const albumGridClass =
  "grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8";

/** The albums of a music library as a grid of square covers, page by page. */
export function MusicLibraryGrid({ albums: pages }: MusicLibraryGridProps) {
  const { t } = useTranslation(musicNamespace);
  const albums = useLibraryAlbums(pages);
  if (albums.status === "pending") {
    return (
      <div role="status" className="w-40">
        <Skeleton label={t("albums.loading")} className="aspect-square w-full rounded-md" />
      </div>
    );
  }
  if (albums.status === "error") {
    return (
      <div className="flex flex-wrap items-center gap-4">
        <p role="alert" className="text-text-muted">
          {t("albums.loadFailed")}
        </p>
        <button type="button" onClick={albums.retry} className={secondaryButtonClass}>
          {t("albums.retry")}
        </button>
      </div>
    );
  }
  if (albums.albums.length === 0) {
    return (
      <EmptyState
        icon={<LibraryKindIcon kind="music" className="size-10" />}
        title={t("albums.empty.title")}
        description={t("albums.empty.hint")}
      />
    );
  }
  return <AlbumList albums={albums} />;
}

function AlbumList({ albums }: { readonly albums: LibraryAlbums }) {
  const { t } = useTranslation(musicNamespace);
  return (
    <>
      <ul aria-label={t("albums.label")} className={albumGridClass}>
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
            loadMore: t("albums.loadMore"),
            loadingMore: t("albums.loadingMore"),
            failed: t("albums.loadMoreFailed"),
            retry: t("albums.retry"),
          }}
        />
      )}
    </>
  );
}
