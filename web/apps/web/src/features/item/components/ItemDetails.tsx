import { useId, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import type { FileView, ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";
import { DetailLayout } from "./DetailLayout";
import { TrackList } from "./TrackList";

/** Props of {@link ItemDetails}. */
export interface ItemDetailsProps {
  /** Receives the title heading, so focus can move to it after a retry. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly item: ItemView;
  /** Shown above the title, e.g. the links of an episode to its show and season. */
  readonly above?: ReactNode;
  /** Facts before the item's own, e.g. the `S1 · E3` label of an episode. */
  readonly leadFacts?: readonly string[];
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

/**
 * A playable item (movie, episode or extra): header, technical chips and Play, then the track
 * lists and the file. Episodes and extras show a 16:9 still, a movie a poster; either falls
 * back to a neutral placeholder.
 */
export function ItemDetails({ headingRef, item, above, leadFacts, actions }: ItemDetailsProps) {
  const { t } = useTranslation(itemNamespace);
  const aspect = item.kind === "movie" ? "poster" : "still";
  return (
    <article>
      <DetailLayout
        headingRef={headingRef}
        item={item}
        aspect={aspect}
        {...(above === undefined ? {} : { above })}
        {...(leadFacts === undefined ? {} : { leadFacts })}
        actions={actions}
      >
        {item.chips.length > 0 && <TechnicalChips chips={item.chips} />}
      </DetailLayout>
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
