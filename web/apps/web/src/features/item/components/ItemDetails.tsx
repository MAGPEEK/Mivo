import { useId, type Ref } from "react";
import { useTranslation } from "react-i18next";

import type { FileView, ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";
import { PlayAction } from "./PlayAction";
import { TrackList } from "./TrackList";

/** Props of {@link ItemDetails}. */
export interface ItemDetailsProps {
  /** Receives the title heading, so focus can move to it after a retry. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly itemId: string;
  readonly item: ItemView;
}

const factSeparator = " · ";

/**
 * A loaded item: header with backdrop, poster, title, facts, technical chips and Play, then
 * the track lists and the file. Backdrop and poster are neutral placeholders until artwork
 * arrives (milestone 3), so the layout stays the same then.
 */
export function ItemDetails({ headingRef, itemId, item }: ItemDetailsProps) {
  const { t } = useTranslation(itemNamespace);
  return (
    <article>
      <div className="relative isolate overflow-hidden rounded-lg border border-divider bg-surface">
        <div
          aria-hidden="true"
          className="absolute inset-0 -z-10 scale-110 bg-linear-to-br from-surface-raised via-surface to-background blur-2xl"
        />
        <div className="flex flex-col gap-6 p-6 sm:flex-row sm:items-end">
          <div
            aria-hidden="true"
            className="grid aspect-2/3 w-40 shrink-0 place-items-center rounded-md bg-surface-raised text-5xl font-semibold text-text-muted shadow-card"
          >
            {item.initial}
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <h1
              ref={headingRef}
              tabIndex={-1}
              className="text-4xl font-semibold break-words focus-visible:outline-none"
            >
              {item.title}
            </h1>
            {item.facts.length > 0 && (
              <p className="text-text-muted">{item.facts.join(factSeparator)}</p>
            )}
            {item.chips.length > 0 && <TechnicalChips chips={item.chips} />}
            <PlayAction itemId={itemId} play={item.play} />
          </div>
        </div>
      </div>
      <div className="mt-8 grid gap-8 lg:grid-cols-3">
        <TrackList
          heading={t("tracks.audio.heading")}
          tracks={item.audioTracks}
          emptyText={t("tracks.audio.empty")}
        />
        <TrackList
          heading={t("tracks.subtitles.heading")}
          tracks={item.subtitleTracks}
          emptyText={t("tracks.subtitles.empty")}
          hint={t("tracks.subtitles.notPlayedYet")}
        />
        {item.file !== undefined && <FileSection file={item.file} />}
      </div>
    </article>
  );
}

function TechnicalChips({ chips }: { readonly chips: readonly string[] }) {
  const { t } = useTranslation(itemNamespace);
  return (
    <ul aria-label={t("chips")} className="flex flex-wrap gap-2">
      {chips.map((chip, position) => (
        <li
          // Chips are derived per render and never reorder, so the position is a stable key.
          key={position}
          className="rounded-sm border border-divider bg-surface-raised px-2.5 py-1 text-sm font-medium text-text"
        >
          {chip}
        </li>
      ))}
    </ul>
  );
}

function FileSection({ file }: { readonly file: FileView }) {
  const { t } = useTranslation(itemNamespace);
  const headingId = useId();
  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId} className="text-lg font-semibold">
        {t("file.heading")}
      </h2>
      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <dt className="text-text-muted">{t("file.name")}</dt>
        <dd className="min-w-0 break-all">{file.name}</dd>
        <dt className="text-text-muted">{t("file.size")}</dt>
        <dd>{file.size}</dd>
      </dl>
    </section>
  );
}
