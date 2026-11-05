import type { ReactNode, Ref } from "react";

import { Poster, type PosterAspect } from "../../../shared/ui";
import { artworkOf, backdropOf } from "../lib/artwork";
import type { ItemView } from "../lib/itemView";
import { Backdrop } from "./Backdrop";
import { PlayAction } from "./PlayAction";
import { Summary } from "./Summary";
import { TitleBlock } from "./TitleBlock";

/** Props of {@link DetailLayout}. */
export interface DetailLayoutProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly item: ItemView;
  /** `poster` for movies, shows and seasons, `still` for episodes and extras, `square` for albums. */
  readonly aspect: PosterAspect;
  /** Shown above the title, e.g. the links to the show and season. */
  readonly above?: ReactNode;
  /** Facts that come before the item's own, e.g. the `S1 · E3` label. */
  readonly leadFacts?: readonly string[];
  /** Shown below the summary, e.g. the technical chips. */
  readonly children?: ReactNode;
  /** Shown next to Play, e.g. the overflow menu with the match actions. */
  readonly actions?: ReactNode;
}

// Width of the artwork column per shape and the matching `sizes`; a still is wider because a
// 16:9 frame at poster width would be too small to recognise (prototype: 260 px and 420 px).
const artworkColumns: Readonly<Record<PosterAspect, { className: string; sizes: string }>> = {
  poster: { className: "w-40 sm:w-56", sizes: "(min-width: 40rem) 14rem, 10rem" },
  still: { className: "w-full sm:w-96", sizes: "(min-width: 40rem) 24rem, 100vw" },
  square: { className: "w-40 sm:w-56", sizes: "(min-width: 40rem) 14rem, 10rem" },
};

/**
 * The shared top of every detail page (DT-01…03, DT-06): blurred backdrop, artwork column with
 * the content-rating badge, title block, Play and actions, tagline and clamped summary. The
 * artwork is decorative, the heading names the item.
 */
export function DetailLayout({
  headingRef,
  item,
  aspect,
  above,
  leadFacts,
  children,
  actions,
}: DetailLayoutProps) {
  const column = artworkColumns[aspect];
  return (
    <div className="relative isolate overflow-hidden rounded-lg border border-divider">
      <div className="flex flex-col gap-6 p-6 sm:flex-row sm:items-start lg:gap-8 lg:p-8">
        <div className={`relative shrink-0 ${column.className}`}>
          <Poster
            imageRef={artworkOf(item, aspect)}
            aspect={aspect}
            alt=""
            sizes={column.sizes}
            className="w-full rounded-lg shadow-card-hover"
          />
          {item.contentRating !== undefined && (
            // The title block names the rating for assistive technology; this is the visual cue.
            <span
              aria-hidden="true"
              className="absolute top-2 left-2 rounded-sm border border-text-muted bg-scrim/75 px-1.5 text-xs leading-5 font-bold text-text"
            >
              {item.contentRating}
            </span>
          )}
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <TitleBlock
            headingRef={headingRef}
            item={item}
            above={above}
            {...(leadFacts === undefined ? {} : { leadFacts })}
          />
          <div className="flex flex-wrap items-start gap-3">
            <PlayAction play={item.play} />
            {actions}
          </div>
          <Summary tagline={item.tagline} summary={item.summary} />
          {children}
        </div>
      </div>
      {/* Last in the DOM but painted below (-z-10): the artwork stays the first image. */}
      <Backdrop imageRef={backdropOf(item, aspect)} />
    </div>
  );
}
