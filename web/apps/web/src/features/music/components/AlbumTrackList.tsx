import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import {
  Badge,
  PlayIcon,
  secondaryButtonClass,
  Skeleton,
  stretchedLinkClass,
} from "../../../shared/ui";
import { queueHandoffState } from "../../player";
import { useAlbumTracks, type AlbumTracks } from "../hooks/useAlbumTracks";
import type { DiscGroup, TrackRowModel } from "../lib/albumTracks";
import { musicNamespace } from "../locales";
import { LoadMoreButton } from "./LoadMoreButton";

/** Props of {@link AlbumTrackList}. */
export interface AlbumTrackListProps {
  readonly albumId: string;
}

/**
 * The tracks of an album, grouped by disc when it has several. A row plays its track in the
 * player; a track whose file cannot be played is listed without Play.
 */
export function AlbumTrackList({ albumId }: AlbumTrackListProps) {
  const { t } = useTranslation(musicNamespace);
  const headingId = useId();
  const tracks = useAlbumTracks(albumId);
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="mb-4 text-lg font-semibold">
        {t("tracks.label")}
      </h2>
      <TrackListBody tracks={tracks} />
    </section>
  );
}

function TrackListBody({ tracks }: { readonly tracks: AlbumTracks }) {
  const { t } = useTranslation(musicNamespace);
  if (tracks.status === "pending") {
    return (
      <div role="status" className="w-60">
        <Skeleton label={t("tracks.loading")} className="h-8 w-full rounded-md" />
      </div>
    );
  }
  if (tracks.status === "error") {
    return (
      <div className="flex flex-wrap items-center gap-4">
        <p role="alert" className="text-text-muted">
          {t("tracks.loadFailed")}
        </p>
        <button type="button" onClick={tracks.retry} className={secondaryButtonClass}>
          {t("tracks.retry")}
        </button>
      </div>
    );
  }
  if (tracks.discs.length === 0) {
    return <p className="text-text-muted">{t("tracks.empty")}</p>;
  }
  return (
    <>
      {tracks.discs.map((group, position) => (
        // Disc numbers repeat only for a disc whose tracks are split by an untagged one.
        <Disc
          key={`${String(group.disc)}-${String(position)}`}
          group={group}
          titled={tracks.showDiscHeadings}
          queueIds={tracks.queueIds}
        />
      ))}
      {tracks.hasMore && (
        <LoadMoreButton
          paged={tracks}
          texts={{
            loadMore: t("tracks.loadMore"),
            loadingMore: t("tracks.loadingMore"),
            failed: t("tracks.loadMoreFailed"),
            retry: t("tracks.retry"),
          }}
        />
      )}
    </>
  );
}

interface DiscProps {
  readonly group: DiscGroup;
  /** Whether the disc gets a heading: only when the album has several. */
  readonly titled: boolean;
  /** The album's playable tracks: the queue a click on a row plays from there. */
  readonly queueIds: readonly string[];
}

function Disc({ group, titled, queueIds }: DiscProps) {
  const { t } = useTranslation(musicNamespace);
  const headingId = useId();
  const name =
    group.disc === undefined ? t("tracks.discUnknown") : t("tracks.disc", { number: group.disc });
  return (
    <div className="mb-4">
      {titled && (
        <h3
          id={headingId}
          className="border-b border-divider px-2 pt-3 pb-1.5 text-xs font-semibold tracking-wide text-text-muted uppercase"
        >
          {name}
        </h3>
      )}
      <ol aria-labelledby={titled ? headingId : undefined} className="flex flex-col">
        {group.tracks.map((track) => (
          <li key={track.id}>
            <TrackRow track={track} queueIds={queueIds} />
          </li>
        ))}
      </ol>
    </div>
  );
}

function TrackRow({
  track,
  queueIds,
}: {
  readonly track: TrackRowModel;
  readonly queueIds: readonly string[];
}) {
  const { t } = useTranslation(musicNamespace);
  return (
    <div className="group relative grid grid-cols-[2.75rem_1fr_auto] items-center gap-3 rounded-sm border-b border-divider px-2 py-2 hover:bg-surface focus-within:bg-surface">
      <span className="relative grid h-8 place-items-center text-sm text-text-muted tabular-nums">
        <span
          className={track.available ? "group-focus-within:invisible group-hover:invisible" : ""}
        >
          {track.number}
        </span>
        {track.available && (
          <PlayIcon className="invisible absolute size-5 text-text group-focus-within:visible group-hover:visible" />
        )}
      </span>
      <span className="flex min-w-0 items-center gap-2">
        {track.available ? (
          <Link
            to="/items/$itemId/play"
            params={{ itemId: track.id }}
            state={queueHandoffState({ ids: queueIds, shuffle: false })}
            aria-label={t("tracks.play", { title: track.title })}
            className={`truncate font-medium ${stretchedLinkClass}`}
          >
            {track.title}
          </Link>
        ) : (
          <>
            <span className="truncate font-medium text-text-muted">{track.title}</span>
            <Badge>{t("tracks.unavailable")}</Badge>
          </>
        )}
      </span>
      <span className="text-sm text-text-muted tabular-nums">{track.duration}</span>
    </div>
  );
}
