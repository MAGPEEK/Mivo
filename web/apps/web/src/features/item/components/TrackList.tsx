import { useId } from "react";
import { useTranslation } from "react-i18next";

import type { TrackView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/** Props of {@link TrackList}. */
export interface TrackListProps {
  readonly heading: string;
  readonly tracks: readonly TrackView[];
  /** Shown instead of the list when there are no tracks. */
  readonly emptyText: string;
  /**
   * Optional muted note about the tracks, e.g. that the player ignores them. Shown only when
   * there are tracks; without any it would explain nothing.
   */
  readonly hint?: string;
}

/** A titled list of audio or subtitle tracks with language, title, format and flags. */
export function TrackList({ heading, tracks, emptyText, hint }: TrackListProps) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId} className="text-lg font-semibold">
        {heading}
      </h2>
      {tracks.length === 0 ? (
        <p className="mt-3 text-sm text-text-muted">{emptyText}</p>
      ) : (
        <>
          {hint !== undefined && <p className="mt-1 text-sm text-text-muted">{hint}</p>}
          <ul aria-labelledby={headingId} className="mt-3 divide-y divide-divider">
            {tracks.map((track) => (
              <TrackRow key={track.index} track={track} />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function TrackRow({ track }: { readonly track: TrackView }) {
  const { t } = useTranslation(itemNamespace);
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm">
      <span className="font-medium text-text">{track.languageName}</span>
      {track.title !== undefined && <span className="text-text-muted">{track.title}</span>}
      {track.format !== "" && <span className="text-text-muted">{track.format}</span>}
      {track.isDefault && <TrackFlag label={t("tracks.default")} />}
      {track.isForced && <TrackFlag label={t("tracks.forced")} />}
    </li>
  );
}

function TrackFlag({ label }: { readonly label: string }) {
  return (
    <span className="rounded-sm border border-divider px-1.5 py-0.5 text-xs text-text-muted">
      {label}
    </span>
  );
}
