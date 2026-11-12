import type { SeekPreview } from "../hooks/useSeekPreview";
import { clockTime, needsHours } from "../lib/playbackTime";
import { bubbleLeft, previewImageWidth, tileStyle, timeOnlyHalfWidth } from "../lib/seekPreview";

/** Props of {@link SeekPreviewBubble}. */
export interface SeekPreviewBubbleProps {
  readonly preview: SeekPreview;
  readonly duration: number | undefined;
}

/**
 * The bubble above the scrubber: the trickplay tile of the position and its time, or the time
 * alone while there is no tile. Purely visual: the slider's value text already says the time, so
 * the bubble is hidden from assistive technology.
 */
export function SeekPreviewBubble({ preview, duration }: SeekPreviewBubbleProps) {
  if (preview.seconds === undefined) {
    return null;
  }
  const { tile } = preview;
  const halfWidth = tile === undefined ? timeOnlyHalfWidth : previewImageWidth / 2;
  return (
    <div
      aria-hidden="true"
      data-testid="seek-preview"
      className="pointer-events-none absolute bottom-full mb-2 flex -translate-x-1/2 flex-col items-center gap-1 overflow-hidden rounded-md bg-surface-raised p-1 text-text shadow-overlay"
      style={{ left: bubbleLeft(preview.percent, halfWidth) }}
    >
      {tile === undefined ? null : (
        <div
          data-testid="seek-preview-tile"
          className="rounded-sm bg-background"
          style={tileStyle(
            {
              sheetUrl: tile.sheetUrl,
              x: tile.tile.x,
              y: tile.tile.y,
              tileWidth: tile.tile.width,
              tileHeight: tile.tile.height,
              columns: tile.layout.columns,
            },
            previewImageWidth,
          )}
        />
      )}
      <span className="px-1 text-sm tabular-nums">
        {clockTime(preview.seconds, needsHours(duration))}
      </span>
    </div>
  );
}
