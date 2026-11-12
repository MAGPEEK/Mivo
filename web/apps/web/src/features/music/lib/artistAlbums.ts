import type { ItemSummary } from "@mivo/api-client";

/**
 * The albums among an artist's children, oldest first; albums without a year come last and
 * equal years keep the server's order (array sort is stable). Singles and compilations are not
 * told apart: the API does not say which an album is.
 */
export function sortAlbumsByYear(children: readonly ItemSummary[]): ItemSummary[] {
  const yearOrLast = (album: ItemSummary) => album.year ?? Number.POSITIVE_INFINITY;
  return children
    .filter((child) => child.kind === "album")
    .sort((a, b) => {
      const [yearA, yearB] = [yearOrLast(a), yearOrLast(b)];
      return yearA === yearB ? 0 : yearA < yearB ? -1 : 1;
    });
}
