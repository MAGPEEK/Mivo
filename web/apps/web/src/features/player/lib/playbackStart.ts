import type { ItemDetail } from "@mivo/api-client";
import { restartEndMarginSeconds } from "@mivo/player-core";

/** The file to play and where to start, ready for the first session request. */
export interface PlaybackStart {
  readonly fileId: string;
  /** File position in seconds, inside the file's known duration. */
  readonly startPositionSeconds: number;
}

/**
 * Picks the file to play and clamps the start position. Both come from the URL and are
 * untrusted (task M2-07): without a file ID the item's primary (first) file plays; an ID the
 * item does not list is passed on unchanged for the server to reject. The start keeps
 * `restartEndMarginSeconds` before the end of a known duration, because the server rejects a
 * start at or after the end; without a known duration (or for a file the item does not list)
 * there is no bound to check `t` against, so playback starts at 0. Returns `undefined` when
 * there is nothing to play.
 */
export function resolvePlaybackStart(
  item: ItemDetail,
  fileId: string | undefined,
  startAt: number | undefined,
): PlaybackStart | undefined {
  const file =
    fileId === undefined ? item.files[0] : item.files.find((candidate) => candidate.id === fileId);
  const resolvedFileId = fileId ?? file?.id;
  if (resolvedFileId === undefined) {
    return undefined;
  }
  return {
    fileId: resolvedFileId,
    startPositionSeconds: clampStart(startAt ?? 0, file?.durationSeconds),
  };
}

function clampStart(start: number, duration: number | undefined): number {
  if (!Number.isFinite(start) || start <= 0) {
    return 0;
  }
  if (duration === undefined || !Number.isFinite(duration)) {
    return 0;
  }
  return Math.min(start, Math.max(0, duration - restartEndMarginSeconds));
}
