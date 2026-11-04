import type { ImageRef, ImageWidth } from "./schemas";

const imagesPath = "/api/v1/images/";

/**
 * Every width the server renders (`ImageWidth`), ascending. Pick the smallest one at least as
 * wide as the displayed size times the device pixel ratio.
 */
export const imageWidths = [
  160, 320, 480, 640, 960, 1280, 1920,
] as const satisfies readonly ImageWidth[];

/**
 * Root-relative URL of an artwork image rendered at `width`, for example
 * `/api/v1/images/<id>?width=320&tag=<tag>`. This helper is the only place that builds image
 * URLs (ADR 0027): the tag is opaque and must reach the server unchanged, because only a
 * request with the current tag may be cached for a year.
 */
export function imageUrl(ref: ImageRef, width: ImageWidth): string {
  const query = `width=${encodeURIComponent(String(width))}&tag=${encodeURIComponent(ref.tag)}`;

  return `${imagesPath}${encodeURIComponent(ref.id)}?${query}`;
}

/**
 * `srcset` value offering `ref` in each of `widths` (`<url> <width>w`), ascending and without
 * duplicates; empty for no widths. Pair it with a `sizes` attribute so the browser picks one.
 */
export function imageSrcSet(ref: ImageRef, widths: readonly ImageWidth[]): string {
  const ascending = [...new Set(widths)].sort((left, right) => left - right);

  return ascending.map((width) => `${imageUrl(ref, width)} ${String(width)}w`).join(", ");
}
