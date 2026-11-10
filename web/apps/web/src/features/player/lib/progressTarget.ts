import type { ItemDetail } from "@mivo/api-client";
import { bookPartStart, reportsProgress } from "@mivo/player-core";

/** How the position in the played file becomes the position the server stores. */
export interface ProgressTarget {
  /**
   * Seconds to add to the file position: 0 for a movie or an episode, the start of the part
   * file on the book's timeline for an audiobook of several parts (API `ProgressReport`).
   */
  readonly offsetSeconds: number;
}

/**
 * Whether and how the player reports the position of `fileId` of `item`. `undefined` for kinds
 * without watch state (music, photos, ADR 0032 section 8) and for an audiobook file the item
 * does not list, whose place in the book is unknown.
 */
export function progressTarget(item: ItemDetail, fileId: string): ProgressTarget | undefined {
  if (!reportsProgress(item.kind)) {
    return undefined;
  }
  if (item.kind !== "audiobook") {
    return { offsetSeconds: 0 };
  }
  const offsetSeconds = bookPartStart(item.files, item.chapters ?? [], fileId);
  return offsetSeconds === undefined ? undefined : { offsetSeconds };
}
