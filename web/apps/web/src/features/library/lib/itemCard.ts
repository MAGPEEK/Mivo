import type { ImageRef, ItemSummary } from "@mivo/api-client";

import type { EpisodeNumbers } from "../../../shared/lib/episodeCode";
import type { CardFormat } from "../../../shared/lib/rowScroll";
import type { PosterAspect } from "../../../shared/ui";

import { formatDuration } from "./formatDuration";
import { mediaChipLabel } from "./mediaChip";

/** What a card in the library grid shows for one item. */
export interface ItemCardModel {
  readonly id: string;
  readonly title: string;
  /** True for a show: its card counts seasons instead of describing a file. */
  readonly isShow: boolean;
  /** Release or first-air year such as `2008`; "" when unknown. */
  readonly year: string;
  /** The poster; `undefined` shows the placeholder. */
  readonly poster: ImageRef | undefined;
  /** Number of seasons of a show; 0 for other kinds or when unknown. */
  readonly seasonCount: number;
  /** Clock notation such as `1:30:23`; "" when unknown. */
  readonly duration: string;
  /** Technical chip such as `MKV · H.264 · AAC`; "" when nothing is known. */
  readonly chip: string;
  /** False when the file could not be probed or no media summary exists; true for shows. */
  readonly available: boolean;
  /** True when the item could not be matched with a provider entry (subtle badge). */
  readonly unmatched: boolean;
}

/** The image and box shape of a card in one format. */
export interface CardArtwork {
  readonly imageRef: ImageRef | undefined;
  readonly aspect: PosterAspect;
}

const formatAspects: Readonly<Record<CardFormat, PosterAspect>> = {
  poster: "poster",
  episode: "still",
  square: "square",
};

/**
 * Picks the artwork of `item` for a card `format`: an episode card shows the still (falling
 * back to the poster), the other formats the poster.
 */
export function cardArtwork(item: ItemSummary, format: CardFormat): CardArtwork {
  const imageRef = format === "episode" ? (item.thumb ?? item.poster) : item.poster;
  return { imageRef, aspect: formatAspects[format] };
}

/**
 * The numbers for the `S3 · E9` label of an episode card, or `undefined` for other kinds. The
 * season comes from the row's source: the item summary does not carry it.
 */
export function cardEpisodeNumbers(
  item: ItemSummary,
  seasonNumber: number | undefined,
): EpisodeNumbers | undefined {
  if (item.kind !== "episode") {
    return undefined;
  }
  return { season: seasonNumber, number: item.number, numberEnd: item.numberEnd };
}

/** Derives the card content of `item`. */
export function toItemCardModel(item: ItemSummary): ItemCardModel {
  const { media } = item;
  const isShow = item.kind === "show";
  return {
    id: item.id,
    title: item.title,
    isShow,
    year: item.year === undefined ? "" : String(item.year),
    poster: item.poster,
    seasonCount: isShow ? (item.childCount ?? 0) : 0,
    duration: media?.durationSeconds === undefined ? "" : formatDuration(media.durationSeconds),
    chip: mediaChipLabel(media),
    // A show is a container without a file of its own (API contract: `ItemSummary.media`).
    available: isShow || (media?.available ?? false),
    unmatched: item.matchState === "unmatched",
  };
}
