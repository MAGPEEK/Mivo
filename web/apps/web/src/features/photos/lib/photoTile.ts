import type { ImageRef, ItemSummary } from "@mivo/api-client";

import { formatTakenDate } from "./photoFacts";

/** What a photo tile in a grid shows. */
export interface PhotoTileModel {
  readonly id: string;
  /** Accessible name of the tile: the title and, when known, the capture date. */
  readonly label: string;
  /** The square tile image; undefined shows a placeholder. */
  readonly image: ImageRef | undefined;
  /** The server cannot render the photo: the tile says so instead of showing an image. */
  readonly unsupported: boolean;
  /** Length of a video in seconds, for the duration badge; undefined for a still photo. */
  readonly durationSeconds: number | undefined;
}

/** Turns a photo of a listing into its tile. */
export function toPhotoTile(photo: ItemSummary, language: string): PhotoTileModel {
  const date = photo.takenAt === undefined ? "" : formatTakenDate(photo.takenAt, language);
  const unsupported = photo.thumbnailState === "unsupported";
  return {
    id: photo.id,
    label: date === "" ? photo.title : `${photo.title}, ${date}`,
    image: unsupported ? undefined : photo.square,
    unsupported,
    durationSeconds: photo.media?.durationSeconds,
  };
}
