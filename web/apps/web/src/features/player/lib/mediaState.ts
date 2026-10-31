// The state of the <video> element the control bar shows, as an external store for React's
// useSyncExternalStore: the element is the source of truth, its events say when to read again.

import type { TimeRange, TimeRangesLike, TimelineSession } from "@mivo/player-core";

import { producedRanges, sameRanges } from "./scrubberTrack";

/** What the control bar shows about the media element. */
export interface MediaState {
  readonly paused: boolean;
  readonly muted: boolean;
  /** Volume from 0 to 1. */
  readonly volume: number;
  /** Produced part of an HLS stream on the file timeline; empty for direct play. */
  readonly producedRanges: readonly TimeRange[];
}

/** The part of a media element the store reads. */
export interface MediaStateSource {
  readonly paused: boolean;
  readonly muted: boolean;
  readonly volume: number;
  readonly seekable: TimeRangesLike;
  addEventListener(type: string, listener: () => void): void;
  removeEventListener(type: string, listener: () => void): void;
}

/** Subscribe and snapshot functions for `useSyncExternalStore`. */
export interface MediaStateStore {
  readonly subscribe: (listener: () => void) => () => void;
  readonly getSnapshot: () => MediaState;
}

/** The state before a media element exists. */
export const initialMediaState: MediaState = {
  paused: true,
  muted: false,
  volume: 1,
  producedRanges: [],
};

// Events after which paused, muted, volume or the produced ranges may have changed.
const stateEvents: readonly string[] = [
  "durationchange",
  "emptied",
  "loadeddata",
  "pause",
  "play",
  "progress",
  "timeupdate",
  "volumechange",
];

/** Reads the element; returns `previous` itself when nothing changed. */
export function readMediaState(
  media: MediaStateSource,
  session: TimelineSession | undefined,
  previous: MediaState,
): MediaState {
  const ranges = producedRanges(session, media.seekable);
  const next: MediaState = {
    paused: media.paused,
    muted: media.muted,
    volume: media.volume,
    producedRanges: sameRanges(ranges, previous.producedRanges) ? previous.producedRanges : ranges,
  };
  const unchanged =
    next.paused === previous.paused &&
    next.muted === previous.muted &&
    next.volume === previous.volume &&
    next.producedRanges === previous.producedRanges;
  return unchanged ? previous : next;
}

/** A store over `media` whose ranges map onto the timeline of `session`. */
export function createMediaStateStore(
  media: MediaStateSource | null,
  session: TimelineSession | undefined,
): MediaStateStore {
  let snapshot = initialMediaState;
  return {
    subscribe: (listener) => {
      for (const type of stateEvents) {
        media?.addEventListener(type, listener);
      }
      return () => {
        for (const type of stateEvents) {
          media?.removeEventListener(type, listener);
        }
      };
    },
    getSnapshot: () => {
      if (media !== null) {
        snapshot = readMediaState(media, session, snapshot);
      }
      return snapshot;
    },
  };
}
