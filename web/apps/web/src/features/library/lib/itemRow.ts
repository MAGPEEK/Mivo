import type { ItemSummary } from "@mivo/api-client";
import { resolutionLabel } from "@mivo/player-core";

import { toItemCardModel, type ItemCardModel } from "./itemCard";

/** What a row of the List or Details view shows for one item. */
export interface ItemRowModel extends ItemCardModel {
  /** Resolution class such as `1080p`; "" when unknown or for shows. */
  readonly resolution: string;
}

/** Derives the row content of `item`, reusing the card model (same chip and duration texts). */
export function toItemRowModel(item: ItemSummary): ItemRowModel {
  const { media } = item;
  const resolution = media?.height === undefined ? "" : resolutionLabel(media.height, media.width);
  return { ...toItemCardModel(item), resolution };
}
