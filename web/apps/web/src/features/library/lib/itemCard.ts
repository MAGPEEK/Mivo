import type { ImageRef, ItemSummary } from "@mivo/api-client";

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
  };
}
