import type { ImageRef, ItemSummary } from "@mivo/api-client";

/** What a card in the album grid shows for one album. */
export interface AlbumCardModel {
  readonly id: string;
  readonly title: string;
  /** Release year such as `2019`; "" when unknown. */
  readonly year: string;
  /** The 1:1 cover; `undefined` shows the placeholder. */
  readonly cover: ImageRef | undefined;
  /** The album's artist, whose name the card loads; `undefined` for an album without one. */
  readonly artistId: string | undefined;
}

/** Derives the card content of an album summary. */
export function toAlbumCardModel(album: ItemSummary): AlbumCardModel {
  return {
    id: album.id,
    title: album.title,
    year: album.year === undefined ? "" : String(album.year),
    cover: album.square,
    artistId: album.parentId,
  };
}
