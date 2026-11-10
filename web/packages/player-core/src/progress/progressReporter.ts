// Decides when a player tells the server its position (ADR 0032 section 8): every 10 seconds
// while playing and right away on pause, seek end, stop, ended and page hide. Timers and the
// request are injected, so the web player, the audiobook player and the TV app share it.

import type { StartTimer } from "../profile/probes";

/** Time between two reports while playing. */
export const progressIntervalMilliseconds = 10_000;

/** Shortest time between two reports, whatever triggers them. */
export const progressThrottleMilliseconds = 2_000;

/**
 * What happened in the player. `play` starts the interval; `seeked` reports and keeps the play
 * state; `pause`, `stop`, `ended` and `pagehide` report and stop the interval.
 */
export type ProgressTrigger = "play" | "pause" | "seeked" | "stop" | "ended" | "pagehide";

/** Time for the reporter; player-core has no timer globals. */
export interface ProgressClock {
  /** Milliseconds on any clock; only differences are used, e.g. of `Date.now()`. */
  readonly now: () => number;
  readonly startTimer: StartTimer;
}

/** What {@link createProgressReporter} needs. */
export interface ProgressReporterOptions {
  /**
   * Sends one absolute position in seconds. A rejection (or a throw) means "not stored": the
   * position is sent again with the next report, never earlier and never queued.
   */
  readonly send: (positionSeconds: number) => Promise<void>;
  readonly clock: ProgressClock;
}

/** Reports the playback position of one item; create a new one per item. */
export interface ProgressReporter {
  /**
   * Sets the latest absolute position in seconds (media time plus the session's start
   * position). Call it whenever the position changes; it never sends by itself. Negative and
   * non-finite values are ignored, because the server rejects them.
   */
  update(positionSeconds: number): void;
  /** Tells the reporter what happened; see {@link ProgressTrigger}. */
  notify(trigger: ProgressTrigger): void;
  /** Cancels the timer; nothing is sent afterwards. */
  dispose(): void;
}

class ThrottledProgressReporter implements ProgressReporter {
  private readonly options: ProgressReporterOptions;
  private position: number | undefined;
  /** The position of the last report that has not failed; unchanged positions are skipped. */
  private reportedPosition: number | undefined;
  private lastSentAt: number | undefined;
  private playing = false;
  private cancelTimer: (() => void) | undefined;
  private disposed = false;

  constructor(options: ProgressReporterOptions) {
    this.options = options;
  }

  update(positionSeconds: number): void {
    if (Number.isFinite(positionSeconds) && positionSeconds >= 0) {
      this.position = positionSeconds;
    }
  }

  notify(trigger: ProgressTrigger): void {
    if (this.disposed) {
      return;
    }
    if (trigger === "play") {
      this.playing = true;
      // A pending timer is a report held back by the throttle; it starts the interval itself.
      if (this.cancelTimer === undefined) {
        this.schedule(progressIntervalMilliseconds);
      }
      return;
    }
    if (trigger !== "seeked") {
      this.playing = false;
    }
    this.reportSoon();
  }

  dispose(): void {
    this.disposed = true;
    this.clearTimer();
  }

  /** Reports now, or at the end of the throttle window when a report was sent just before. */
  private reportSoon(): void {
    const wait = this.throttleRemaining();
    if (wait > 0) {
      this.schedule(wait);
      return;
    }
    this.clearTimer();
    this.reportAndContinue();
  }

  private throttleRemaining(): number {
    if (this.lastSentAt === undefined) {
      return 0;
    }
    return this.lastSentAt + progressThrottleMilliseconds - this.options.clock.now();
  }

  private reportAndContinue(): void {
    this.sendIfChanged();
    if (this.playing) {
      this.schedule(progressIntervalMilliseconds);
    }
  }

  /** One timer at most: a new one replaces the pending one, so nothing piles up. */
  private schedule(milliseconds: number): void {
    this.clearTimer();
    this.cancelTimer = this.options.clock.startTimer(milliseconds, () => {
      this.cancelTimer = undefined;
      this.reportAndContinue();
    });
  }

  private clearTimer(): void {
    this.cancelTimer?.();
    this.cancelTimer = undefined;
  }

  private sendIfChanged(): void {
    const position = this.position;
    if (position === undefined || position === this.reportedPosition) {
      return;
    }
    this.reportedPosition = position;
    this.lastSentAt = this.options.clock.now();
    const forget = () => {
      // Only the latest report counts: a newer one has replaced this position already.
      if (this.reportedPosition === position) {
        this.reportedPosition = undefined;
      }
    };
    try {
      this.options.send(position).then(undefined, forget);
    } catch {
      forget();
    }
  }
}

/**
 * Creates the reporter of one item's playback position. It reports every
 * {@link progressIntervalMilliseconds} while playing and at once on every other trigger,
 * skips a position it has reported already and never sends twice within
 * {@link progressThrottleMilliseconds}: a report inside that window waits for its end. A failed
 * report is not repeated by itself; the next report carries the position.
 */
export function createProgressReporter(options: ProgressReporterOptions): ProgressReporter {
  return new ThrottledProgressReporter(options);
}
