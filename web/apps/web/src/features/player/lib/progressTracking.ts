// Feeds a player-core progress reporter from the playback engine, the media element and the
// page (task M7-35). Framework-free, so it is tested with the engine harness; the hook
// useProgressReporting only creates and disposes it.

import type { ProgressReporter, ProgressTrigger } from "@mivo/player-core";

import type { PlaybackSnapshot } from "./playbackEngine";

/** Media element events that trigger a report. */
export type ProgressMediaEvent = "play" | "pause" | "seeked" | "ended";

/** The part of the media element the tracking uses; a `<video>` element satisfies it. */
export interface ProgressMedia {
  readonly paused: boolean;
  addEventListener(type: ProgressMediaEvent, listener: () => void): void;
  removeEventListener(type: ProgressMediaEvent, listener: () => void): void;
}

/** The part of the playback engine the tracking uses. */
export interface ProgressSource {
  readonly subscribe: (listener: () => void) => () => void;
  readonly getSnapshot: () => PlaybackSnapshot;
  /** The displayed file position right now (see `PlaybackEngine.currentPosition`). */
  currentPosition(): number;
}

/** The part of `window` the tracking uses. */
export interface ProgressPage {
  addEventListener(type: "pagehide", listener: () => void): void;
  removeEventListener(type: "pagehide", listener: () => void): void;
}

/** What {@link trackProgress} connects. */
export interface ProgressTracking {
  readonly source: ProgressSource;
  readonly media: ProgressMedia;
  readonly page: ProgressPage;
  readonly reporter: ProgressReporter;
  /** Seconds between the file position and the reported one (see `progressTarget`). */
  readonly offsetSeconds: number;
}

const mediaTriggers: Readonly<Record<ProgressMediaEvent, ProgressTrigger>> = {
  play: "play",
  pause: "pause",
  seeked: "seeked",
  ended: "ended",
};

const mediaEvents: readonly ProgressMediaEvent[] = ["play", "pause", "seeked", "ended"];

/**
 * Reports the engine's position through `reporter` until the returned function is called,
 * which sends the last position (`stop`) and ends the tracking.
 *
 * The position always comes from the engine, never from the element alone: the engine knows
 * the session's offset and keeps the position while the element is empty (a restart after a
 * seek outside the produced part, a hidden page), where the element's clock reads 0. A new
 * session that starts playing after such a restart counts as the end of the seek.
 */
export function trackProgress(tracking: ProgressTracking): () => void {
  const { source, media, page, reporter, offsetSeconds } = tracking;
  const update = (position: number) => {
    // Without a session there is nothing played yet, or the engine was closed (position 0).
    if (source.getSnapshot().session !== undefined) {
      reporter.update(offsetSeconds + position);
    }
  };
  const report = (trigger: ProgressTrigger) => {
    update(source.currentPosition());
    reporter.notify(trigger);
  };
  let status = source.getSnapshot().status;
  let hasPlayed = status === "playing";
  const unsubscribe = source.subscribe(() => {
    const snapshot = source.getSnapshot();
    update(snapshot.position);
    const startedPlaying = status !== "playing" && snapshot.status === "playing";
    status = snapshot.status;
    if (startedPlaying && hasPlayed) {
      reporter.notify("seeked");
    }
    hasPlayed = hasPlayed || startedPlaying;
  });
  const mediaListeners = mediaEvents.map((event) => {
    const listener = () => {
      report(mediaTriggers[event]);
    };
    media.addEventListener(event, listener);
    return { event, listener };
  });
  const onPageHide = () => {
    report("pagehide");
  };
  page.addEventListener("pagehide", onPageHide);
  update(source.getSnapshot().position);
  if (!media.paused) {
    reporter.notify("play");
  }
  return () => {
    report("stop");
    reporter.dispose();
    unsubscribe();
    for (const { event, listener } of mediaListeners) {
      media.removeEventListener(event, listener);
    }
    page.removeEventListener("pagehide", onPageHide);
  };
}
