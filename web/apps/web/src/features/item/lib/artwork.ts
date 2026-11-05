import type { ImageRef } from "@mivo/api-client";

import type { PosterAspect } from "../../../shared/ui";
import type { ItemView } from "./itemView";

/** The images of an item the detail layout picks from. */
export type ItemArtwork = Pick<ItemView, "poster" | "still" | "square" | "backdrop">;

/** The image that fills the artwork column of each shape. */
export function artworkOf(item: ItemArtwork, aspect: PosterAspect): ImageRef | undefined {
  switch (aspect) {
    case "poster":
      return item.poster;
    case "still":
      return item.still;
    case "square":
      return item.square;
  }
}

/**
 * What fills the backdrop behind the title (DT-01): the backdrop artwork, else the item's own
 * artwork blurred in its place, else nothing and the layout shows a plain background.
 */
export function backdropOf(item: ItemArtwork, aspect: PosterAspect): ImageRef | undefined {
  return item.backdrop ?? artworkOf(item, aspect) ?? item.poster;
}
