// Maps between the media element's clock and the position in the file (ADR 0024). An HLS
// session that starts at position p produces output whose timestamps begin at 0, so the player
// shows `p + currentTime` and draws its scrubber over the probed duration of the whole file. A
// direct-play session serves the original file, whose clock already is the file position.

import type { PlaybackSession } from "../streamInfo/playbackSession";
import type { TimeRange } from "./timeRanges";

/** The part of a session the timeline needs. */
export type TimelineSession = Pick<
  PlaybackSession,
  "delivery" | "startPositionSeconds" | "durationSeconds"
>;

/** The probed duration of the file in seconds, or `undefined` when it is unknown or unusable. */
export function knownDuration(session: TimelineSession): number | undefined {
  const duration = session.durationSeconds;
  return duration !== undefined && Number.isFinite(duration) && duration >= 0
    ? duration
    : undefined;
}

/**
 * Seconds between the file position and the media element's clock: 0 for a progressive file,
 * the actual start position for streams that start their timestamps at 0 (HLS and later
 * deliveries).
 */
export function mediaClockOffset(session: TimelineSession): number {
  if (session.delivery === "file") {
    return 0;
  }
  const start = session.startPositionSeconds;
  return Number.isFinite(start) && start > 0 ? start : 0;
}

/**
 * Clamps a file position to `[0, duration]` (`[0, ∞)` while the duration is unknown). `NaN`
 * counts as 0 and `Infinity` as the end of the file; without a known duration there is no end,
 * so it falls back to 0, the only position every file has.
 */
export function clampPosition(session: TimelineSession, position: number): number {
  if (Number.isNaN(position) || position <= 0) {
    return 0;
  }
  const duration = knownDuration(session);
  if (duration !== undefined) {
    return Math.min(position, duration);
  }
  return Number.isFinite(position) ? position : 0;
}

/**
 * The position to display for the media element's `currentTime`, clamped to the file. Before
 * the element knows its time (`NaN`) it shows where the session starts.
 */
export function displayedPosition(session: TimelineSession, currentTime: number): number {
  const elementTime = Number.isNaN(currentTime) || currentTime < 0 ? 0 : currentTime;
  return clampPosition(session, mediaClockOffset(session) + elementTime);
}

/**
 * The media element time for a file position, the inverse of {@link displayedPosition} for
 * local seeks. Positions before the start of an HLS session map to its first frame; use
 * `planSeek` to decide whether the element can reach a position at all.
 */
export function mediaTimeForPosition(session: TimelineSession, position: number): number {
  return Math.max(0, clampPosition(session, position) - mediaClockOffset(session));
}

/**
 * Converts ranges of the media element's clock (for example its `seekable` or `buffered`
 * ranges) into file positions, clamped to the file, to shade the produced part of an HLS
 * stream on the scrubber. Ranges that end up empty are dropped.
 */
export function displayedRanges(
  session: TimelineSession,
  ranges: readonly TimeRange[],
): TimeRange[] {
  return ranges.flatMap((range) => {
    const start = displayedPosition(session, range.start);
    const end = displayedPosition(session, range.end);
    return end > start ? [{ start, end }] : [];
  });
}
