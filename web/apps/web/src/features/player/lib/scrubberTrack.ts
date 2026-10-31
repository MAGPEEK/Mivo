// Geometry of the scrubber (ADR 0024 Notes M2-30): it spans the probed duration of the whole
// file, fills up to the current position and shades the part an HLS stream has produced.

import {
  displayedRanges,
  readTimeRanges,
  type TimeRange,
  type TimeRangesLike,
  type TimelineSession,
} from "@mivo/player-core";

const fullPercent = 100;

/**
 * The produced part of an HLS stream on the file timeline, from the element's `seekable`
 * ranges. A direct-play file has the whole file, so nothing is shaded; neither is anything
 * before the first session.
 */
export function producedRanges(
  session: TimelineSession | undefined,
  seekable: TimeRangesLike,
): TimeRange[] {
  if (session === undefined || session.delivery === "file") {
    return [];
  }
  return displayedRanges(session, readTimeRanges(seekable));
}

/** Whether two range lists are equal, so unchanged ranges keep their object. */
export function sameRanges(first: readonly TimeRange[], second: readonly TimeRange[]): boolean {
  return (
    first.length === second.length &&
    first.every(
      (range, index) => range.start === second[index]?.start && range.end === second[index].end,
    )
  );
}

/** A position on the track as a percentage of the duration, inside `[0, 100]`. */
export function trackPercent(seconds: number, durationSeconds: number | undefined): number {
  if (durationSeconds === undefined || durationSeconds <= 0 || !(seconds > 0)) {
    return 0;
  }
  return Math.min(fullPercent, (seconds / durationSeconds) * fullPercent);
}

/** Where a range sits on the track, in percent of the duration. */
export interface TrackSpan {
  readonly startPercent: number;
  readonly widthPercent: number;
}

/** The span of `range` on a track over `durationSeconds`. */
export function trackSpan(range: TimeRange, durationSeconds: number | undefined): TrackSpan {
  const startPercent = trackPercent(range.start, durationSeconds);
  return {
    startPercent,
    widthPercent: trackPercent(range.end, durationSeconds) - startPercent,
  };
}
