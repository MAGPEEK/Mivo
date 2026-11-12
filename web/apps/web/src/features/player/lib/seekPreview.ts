// Geometry of the seek-bar preview (M4-16d): which file position a pointer is over and how the
// tile of a sprite sheet is cropped with CSS.

/** Width of the preview image on screen, in CSS pixels; tiles are scaled to it. */
export const previewImageWidth = 160;

/** Half the width of the time-only bubble, used to keep it inside the bar. */
export const timeOnlyHalfWidth = 28;

/** The part of an element's box the pointer maths needs. */
export interface TrackBox {
  readonly left: number;
  readonly width: number;
}

/** The file position under `clientX` on a track; clamped to the file, `0` for an empty box. */
export function positionAtPointer(
  clientX: number,
  box: TrackBox,
  durationSeconds: number | undefined,
): number {
  if (durationSeconds === undefined || durationSeconds <= 0 || !(box.width > 0)) {
    return 0;
  }
  const fraction = Math.min(1, Math.max(0, (clientX - box.left) / box.width));
  return fraction * durationSeconds;
}

/** The part of a sheet to show and the sheet's size, as the manifest states them. */
export interface TileCrop {
  readonly sheetUrl: string;
  readonly x: number;
  readonly y: number;
  readonly tileWidth: number;
  readonly tileHeight: number;
  readonly columns: number;
}

/** Inline style of the preview element: one tile of the sheet, scaled to `displayWidth`. */
export function tileStyle(crop: TileCrop, displayWidth: number): Record<string, string> {
  const scale = displayWidth / crop.tileWidth;
  return {
    width: `${String(displayWidth)}px`,
    height: `${String(Math.round(crop.tileHeight * scale))}px`,
    backgroundImage: `url("${crop.sheetUrl}")`,
    // A sheet with fewer rows (the last one) keeps its aspect ratio with `auto`.
    backgroundSize: `${String(crop.columns * crop.tileWidth * scale)}px auto`,
    backgroundPosition: `${String(-crop.x * scale)}px ${String(-crop.y * scale)}px`,
    backgroundRepeat: "no-repeat",
  };
}

/** CSS `left` that centres a bubble of `halfWidth` on `percent` and keeps it inside the bar. */
export function bubbleLeft(percent: number, halfWidth: number): string {
  return `clamp(${String(halfWidth)}px, ${String(percent)}%, calc(100% - ${String(halfWidth)}px))`;
}
