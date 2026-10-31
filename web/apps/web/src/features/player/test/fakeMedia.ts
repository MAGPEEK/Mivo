// A media element without media: records what the engine does and lets tests fire the events
// a real <video> element would.

import type { StartTimer, TimeRange, TimeRangesLike } from "@mivo/player-core";

import type { HlsFactory, HlsPlayer, HlsStreamError } from "../lib/hlsPlayer";
import type { PlaybackMedia, PlaybackMediaEvent } from "../lib/playbackPorts";

function timeRanges(ranges: readonly TimeRange[]): TimeRangesLike {
  return {
    length: ranges.length,
    start: (index) => ranges[index]?.start ?? Number.NaN,
    end: (index) => ranges[index]?.end ?? Number.NaN,
  };
}

/** Stands in for a `<video>` element. */
export class FakeMedia extends EventTarget implements PlaybackMedia {
  src = "";
  currentTime = 0;
  paused = true;
  error: { readonly code: number } | null = null;
  seekableRanges: TimeRange[] = [];
  bufferedRanges: TimeRange[] = [];
  /** Calls of `play`, `load` and `removeAttribute`, oldest first. */
  readonly calls: string[] = [];
  /** Result of the next `play()` calls; a browser rejects without a user gesture. */
  playResult: () => Promise<void> = () => Promise.resolve();

  get seekable(): TimeRangesLike {
    return timeRanges(this.seekableRanges);
  }

  get buffered(): TimeRangesLike {
    return timeRanges(this.bufferedRanges);
  }

  play(): Promise<void> {
    this.calls.push("play");
    this.paused = false;
    return this.playResult();
  }

  load(): void {
    this.calls.push("load");
    this.paused = true;
  }

  removeAttribute(name: string): void {
    this.calls.push(`removeAttribute ${name}`);
    if (name === "src") {
      this.src = "";
    }
  }

  /** Fires `type` like the browser would. */
  emit(type: PlaybackMediaEvent): void {
    this.dispatchEvent(new Event(type));
  }

  /** Sets `currentTime` and fires `timeupdate`. */
  playTo(time: number): void {
    this.currentTime = time;
    this.emit("timeupdate");
  }

  /** Sets `error` to a `MediaError` code and fires `error`. */
  fail(code: number): void {
    this.error = { code };
    this.emit("error");
  }
}

/** One hls.js stand-in created by {@link createFakeHls}. */
export interface FakeHlsPlayer extends HlsPlayer<FakeMedia> {
  /** Reports an error to the engine like hls.js' `ERROR` event. */
  readonly report: (error: HlsStreamError) => void;
}

/** A factory of hls.js stand-ins and the log of what was done with them. */
export interface FakeHls {
  readonly factory: HlsFactory<FakeMedia>;
  readonly players: readonly FakeHlsPlayer[];
  /** `create 1`, `attach 1`, `load 1 <url>`, `recover 1`, `destroy 1`, ... across all players. */
  readonly log: readonly string[];
}

/** Creates hls.js stand-ins; `failToLoad` makes the factory reject like a missing chunk. */
export function createFakeHls(failToLoad = false): FakeHls {
  const players: FakeHlsPlayer[] = [];
  const log: string[] = [];
  const factory: HlsFactory<FakeMedia> = (onError) => {
    if (failToLoad) {
      return Promise.reject(new Error("chunk failed to load"));
    }
    const number = String(players.length + 1);
    log.push(`create ${number}`);
    const player: FakeHlsPlayer = {
      attachMedia: () => log.push(`attach ${number}`),
      recoverMediaError: () => log.push(`recover ${number}`),
      loadSource: (url) => log.push(`load ${number} ${url}`),
      destroy: () => log.push(`destroy ${number}`),
      report: (error) => {
        onError(error, player);
      },
    };
    players.push(player);
    return Promise.resolve(player);
  };
  return { factory, players, log };
}

/** A timer that has not fired yet. */
export interface PendingTimer {
  readonly milliseconds: number;
  readonly fire: () => void;
}

/** Timers that only fire when the test says so. */
export interface ManualTimers {
  readonly startTimer: StartTimer;
  /** Timers started and neither fired nor cancelled, oldest first. */
  readonly pending: () => readonly PendingTimer[];
}

/** Creates {@link ManualTimers}. */
export function createManualTimers(): ManualTimers {
  const timers: { milliseconds: number; callback: () => void; active: boolean }[] = [];
  const startTimer: StartTimer = (milliseconds, callback) => {
    const timer = { milliseconds, callback, active: true };
    timers.push(timer);
    return () => {
      timer.active = false;
    };
  };
  const pending = () =>
    timers
      .filter((timer) => timer.active)
      .map((timer) => ({
        milliseconds: timer.milliseconds,
        fire: () => {
          timer.active = false;
          timer.callback();
        },
      }));
  return { startTimer, pending };
}
