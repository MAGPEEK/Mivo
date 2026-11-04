import type { ReactNode, Ref } from "react";
import { useTranslation } from "react-i18next";

import { Poster, type PosterAspect } from "../../../shared/ui";
import type { ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";
import { PlayAction } from "./PlayAction";

/** Props of {@link ItemHeader}. */
export interface ItemHeaderProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly item: ItemView;
  /** `poster` for movies, shows and seasons, `still` for episodes and extras. */
  readonly aspect: PosterAspect;
  /** Shown above the title, e.g. the links to the show and season. */
  readonly above?: ReactNode;
  /** Facts that come before the item's own, e.g. the `S1 · E3` label. */
  readonly leadFacts?: readonly string[];
  /** Shown between the summary and Play, e.g. the technical chips. */
  readonly children?: ReactNode;
}

const factSeparator = " · ";
// The artwork box is w-40 (10rem) as a poster and w-64 (16rem) as a still at every breakpoint.
const artworkBoxes: Readonly<Record<PosterAspect, { className: string; sizes: string }>> = {
  poster: { className: "w-40", sizes: "10rem" },
  still: { className: "w-64", sizes: "16rem" },
};

/**
 * The top of every detail page: artwork, title, facts, genres, summary and Play. The artwork is
 * decorative, the heading names the item. The backdrop stays a neutral gradient until
 * backdrop artwork is shown (milestone 5).
 */
export function ItemHeader({
  headingRef,
  item,
  aspect,
  above,
  leadFacts = [],
  children,
}: ItemHeaderProps) {
  const { t } = useTranslation(itemNamespace);
  const facts = [...leadFacts, ...item.facts];
  const box = artworkBoxes[aspect];
  return (
    <div className="relative isolate overflow-hidden rounded-lg border border-divider bg-surface">
      <div
        aria-hidden="true"
        className="absolute inset-0 -z-10 scale-110 bg-linear-to-br from-surface-raised via-surface to-background blur-2xl"
      />
      <div className="flex flex-col gap-6 p-6 sm:flex-row sm:items-end">
        <Poster
          imageRef={aspect === "poster" ? item.poster : item.still}
          aspect={aspect}
          alt=""
          sizes={box.sizes}
          className={`${box.className} shrink-0 rounded-md shadow-card`}
        />
        <div className="flex min-w-0 flex-col gap-4">
          {above}
          <h1
            ref={headingRef}
            tabIndex={-1}
            className="text-4xl font-semibold break-words focus-visible:outline-none"
          >
            {item.title}
          </h1>
          {facts.length > 0 && <p className="text-text-muted">{facts.join(factSeparator)}</p>}
          {item.genres.length > 0 && (
            <ul aria-label={t("genres")} className="flex flex-wrap gap-2 text-sm text-text-muted">
              {item.genres.map((genre) => (
                <li key={genre}>{genre}</li>
              ))}
            </ul>
          )}
          {item.summary !== undefined && <p className="max-w-prose">{item.summary}</p>}
          {children}
          <PlayAction play={item.play} />
        </div>
      </div>
    </div>
  );
}
