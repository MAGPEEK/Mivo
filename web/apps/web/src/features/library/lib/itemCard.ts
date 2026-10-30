import type { ItemSummary } from "@mivo/api-client";

import { formatDuration } from "./formatDuration";
import { mediaChipLabel } from "./mediaChip";

/** What a card in the library grid shows for one item. */
export interface ItemCardModel {
  readonly id: string;
  readonly title: string;
  /** First letter of the title, upper case, for the placeholder poster; "" for no title. */
  readonly initial: string;
  /** Clock notation such as `1:30:23`; "" when unknown. */
  readonly duration: string;
  /** Technical chip such as `MKV · H.264 · AAC`; "" when nothing is known. */
  readonly chip: string;
  /** False when the file could not be probed or no media summary exists. */
  readonly available: boolean;
}

/** Derives the card content of `item`. */
export function toItemCardModel(item: ItemSummary): ItemCardModel {
  const { media } = item;
  // Array.from splits by code point, so an emoji or astral letter is not cut in half.
  const [firstCharacter = ""] = Array.from(item.title.trim());
  return {
    id: item.id,
    title: item.title,
    initial: firstCharacter.toLocaleUpperCase(),
    duration: media?.durationSeconds === undefined ? "" : formatDuration(media.durationSeconds),
    chip: mediaChipLabel(media),
    available: media?.available ?? false,
  };
}
