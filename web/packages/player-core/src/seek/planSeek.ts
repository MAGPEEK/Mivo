// Seeking by restart (ADR 0024): a seek the media element can serve from what it already has is
// local; any other seek asks the server for a new session at the target position.

import {
  clampPosition,
  knownDuration,
  mediaClockOffset,
  type TimelineSession,
} from "../timeline/timeline";
import { rangeContains, type TimeRange } from "../timeline/timeRanges";

/**
 * Distance a restart keeps from the end of the file, in seconds. The server rejects a start
 * position at or after the duration (400), and a stream starting in the last fraction of a
 * second would end before it shows a frame.
 */
export const restartEndMarginSeconds = 1;

/** What the media element can play at the moment of the seek. */
export interface SeekState {
  readonly session: TimelineSession;
  /** The element's `seekable` ranges, in its own clock (see `readTimeRanges`). */
  readonly seekable: readonly TimeRange[];
  /** The element's `buffered` ranges, in its own clock. */
  readonly buffered: readonly TimeRange[];
}

/** Set the element's `currentTime` to `time`. */
export interface LocalSeek {
  readonly kind: "local";
  readonly time: number;
}

/** Create a new session at file position `position` that replaces the current one. */
export interface RestartSeek {
  readonly kind: "restart";
  readonly position: number;
}

/** How to reach a seek target. */
export type SeekPlan = LocalSeek | RestartSeek;

/**
 * Plans a seek to the file position `target` (clamped to the file). Direct play always seeks
 * locally because the element has the whole file. A stream (HLS) seeks locally only when the
 * target lies inside the seekable or buffered ranges passed in; otherwise it restarts, at most
 * {@link restartEndMarginSeconds} before the end of the file.
 */
export function planSeek(target: number, state: SeekState): SeekPlan {
  const { session } = state;
  const position = clampPosition(session, target);
  if (session.delivery === "file") {
    return { kind: "local", time: position };
  }
  const time = position - mediaClockOffset(session);
  const playable = [...state.seekable, ...state.buffered];
  if (time >= 0 && playable.some((range) => rangeContains(range, time))) {
    return { kind: "local", time };
  }
  return { kind: "restart", position: restartPosition(session, position) };
}

function restartPosition(session: TimelineSession, position: number): number {
  const duration = knownDuration(session);
  if (duration === undefined) {
    return position;
  }
  return Math.min(position, Math.max(0, duration - restartEndMarginSeconds));
}
