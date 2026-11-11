import { beforeEach, describe, expect, it } from "vitest";

import {
  createProgressReporter,
  progressIntervalMilliseconds,
  progressThrottleMilliseconds,
  type ProgressClock,
  type ProgressReporter,
} from "./progressReporter";

interface Harness {
  readonly reporter: ProgressReporter;
  /** Positions sent, oldest first. */
  readonly sent: number[];
  /** Makes the following sends fail (`true`) or succeed again. */
  readonly failSends: (fail: boolean) => void;
}

// Fake timers: player-core has no timer globals, so the injected clock is the fake.
interface FakeTimer {
  readonly at: number;
  readonly onTimeout: () => void;
}

let now = 0;
let timers: FakeTimer[] = [];

const fakeTimerClock: ProgressClock = {
  now: () => now,
  startTimer: (milliseconds, onTimeout) => {
    const timer = { at: now + milliseconds, onTimeout };
    timers.push(timer);
    return () => {
      timers = timers.filter((candidate) => candidate !== timer);
    };
  },
};

/** Moves the clock forward and fires the timers that are due, oldest first. */
function advanceTimersByTime(milliseconds: number): void {
  const end = now + milliseconds;
  for (;;) {
    const due = timers.filter((timer) => timer.at <= end).sort((a, b) => a.at - b.at)[0];
    if (due === undefined) {
      break;
    }
    timers = timers.filter((candidate) => candidate !== due);
    now = due.at;
    due.onTimeout();
  }
  now = end;
}

/** Like {@link advanceTimersByTime}, and lets the answers of the sends arrive. */
async function advanceTimersByTimeAsync(milliseconds: number): Promise<void> {
  advanceTimersByTime(milliseconds);
  await Promise.resolve();
  await Promise.resolve();
}

function timerCount(): number {
  return timers.length;
}

function createHarness(): Harness {
  const sent: number[] = [];
  let failing = false;
  const reporter = createProgressReporter({
    send: (positionSeconds) => {
      sent.push(positionSeconds);
      return failing ? Promise.reject(new Error("offline")) : Promise.resolve();
    },
    clock: fakeTimerClock,
  });
  return {
    reporter,
    sent,
    failSends: (fail) => {
      failing = fail;
    },
  };
}

beforeEach(() => {
  now = 0;
  timers = [];
});

describe("createProgressReporter while playing", () => {
  it("reports every 10 seconds", () => {
    const { reporter, sent } = createHarness();
    reporter.update(0);
    reporter.notify("play");

    expect(sent).toEqual([]);
    reporter.update(10);
    advanceTimersByTime(progressIntervalMilliseconds);
    reporter.update(20);
    advanceTimersByTime(progressIntervalMilliseconds - 1);
    expect(sent).toEqual([10]);
    advanceTimersByTime(1);

    expect(sent).toEqual([10, 20]);
  });

  it("skips a position that has not changed", () => {
    const { reporter, sent } = createHarness();
    reporter.update(42);
    reporter.notify("play");

    advanceTimersByTime(3 * progressIntervalMilliseconds);

    expect(sent).toEqual([42]);
  });

  it("sends nothing before a position is known and ignores unusable positions", () => {
    const { reporter, sent } = createHarness();
    reporter.notify("play");
    reporter.update(Number.NaN);
    reporter.update(-1);
    reporter.update(Number.POSITIVE_INFINITY);

    advanceTimersByTime(progressIntervalMilliseconds);
    reporter.notify("pause");

    expect(sent).toEqual([]);
  });

  it("does not start a second interval for a repeated play", () => {
    const { reporter, sent } = createHarness();
    reporter.update(1);
    reporter.notify("play");
    advanceTimersByTime(progressIntervalMilliseconds / 2);
    reporter.notify("play");
    advanceTimersByTime(progressIntervalMilliseconds / 2);

    expect(sent).toEqual([1]);
    expect(timerCount()).toBe(1);
  });
});

describe("createProgressReporter triggers", () => {
  it.each(["pause", "stop", "ended", "pagehide"] as const)(
    "reports at once on %s and stops the interval",
    (trigger) => {
      const { reporter, sent } = createHarness();
      reporter.notify("play");
      reporter.update(7.5);

      reporter.notify(trigger);
      reporter.update(99);
      advanceTimersByTime(3 * progressIntervalMilliseconds);

      expect(sent).toEqual([7.5]);
      expect(timerCount()).toBe(0);
    },
  );

  it("reports at once when a seek ends and keeps the interval", () => {
    const { reporter, sent } = createHarness();
    reporter.notify("play");
    advanceTimersByTime(4_000);
    reporter.update(600);

    reporter.notify("seeked");
    expect(sent).toEqual([600]);
    reporter.update(609);
    advanceTimersByTime(progressIntervalMilliseconds - 1);
    expect(sent).toEqual([600]);
    advanceTimersByTime(1);

    expect(sent).toEqual([600, 609]);
  });

  it("reports a seek while paused without starting the interval", () => {
    const { reporter, sent } = createHarness();
    reporter.update(300);

    reporter.notify("seeked");
    reporter.update(301);
    advanceTimersByTime(3 * progressIntervalMilliseconds);

    expect(sent).toEqual([300]);
  });

  it("sends nothing after dispose", () => {
    const { reporter, sent } = createHarness();
    reporter.update(5);
    reporter.notify("play");

    reporter.dispose();
    reporter.notify("pause");
    advanceTimersByTime(3 * progressIntervalMilliseconds);

    expect(sent).toEqual([]);
    expect(timerCount()).toBe(0);
  });
});

describe("createProgressReporter throttle", () => {
  it("never sends twice within 2 seconds: a burst of seeks sends the last position once", () => {
    const { reporter, sent } = createHarness();
    reporter.update(100);
    reporter.notify("seeked");
    advanceTimersByTime(500);
    reporter.update(200);
    reporter.notify("seeked");
    advanceTimersByTime(500);
    reporter.update(300);
    reporter.notify("seeked");

    expect(sent).toEqual([100]);
    advanceTimersByTime(progressThrottleMilliseconds - 1_000 - 1);
    expect(sent).toEqual([100]);
    advanceTimersByTime(1);

    expect(sent).toEqual([100, 300]);
    expect(timerCount()).toBe(0);
  });

  it("holds a pause back that follows a report and then stops", () => {
    const { reporter, sent } = createHarness();
    reporter.update(10);
    reporter.notify("play");
    advanceTimersByTime(progressIntervalMilliseconds);
    advanceTimersByTime(1_000);
    reporter.update(11);

    reporter.notify("pause");
    expect(sent).toEqual([10]);
    advanceTimersByTime(1_000);
    reporter.update(50);
    advanceTimersByTime(3 * progressIntervalMilliseconds);

    expect(sent).toEqual([10, 11]);
  });

  it.each(["stop", "pagehide"] as const)(
    "sends the last position at once on %s inside the throttle window, before dispose",
    (trigger) => {
      const { reporter, sent } = createHarness();
      reporter.update(10);
      reporter.notify("pause");
      advanceTimersByTime(500);
      reporter.update(11);

      reporter.notify(trigger);
      reporter.dispose();
      advanceTimersByTime(3 * progressIntervalMilliseconds);

      expect(sent).toEqual([10, 11]);
      expect(timerCount()).toBe(0);
    },
  );

  it("drops a held-back report when a final one has sent its position", () => {
    const { reporter, sent } = createHarness();
    reporter.update(10);
    reporter.notify("play");
    reporter.notify("seeked");
    advanceTimersByTime(500);
    reporter.update(11);
    reporter.notify("seeked");

    reporter.notify("pagehide");
    reporter.update(50);
    advanceTimersByTime(3 * progressIntervalMilliseconds);

    expect(sent).toEqual([10, 11]);
    expect(timerCount()).toBe(0);
  });

  it("starts the interval after a held-back report when playback went on", () => {
    const { reporter, sent } = createHarness();
    reporter.update(10);
    reporter.notify("seeked");
    advanceTimersByTime(1_000);
    reporter.update(11);
    reporter.notify("pause");
    reporter.notify("play");

    advanceTimersByTime(1_000);
    expect(sent).toEqual([10, 11]);
    reporter.update(21);
    advanceTimersByTime(progressIntervalMilliseconds);

    expect(sent).toEqual([10, 11, 21]);
  });
});

describe("createProgressReporter failures", () => {
  it("retries a failed position with the next tick only", async () => {
    const { reporter, sent, failSends } = createHarness();
    failSends(true);
    reporter.update(10);
    reporter.notify("play");
    await advanceTimersByTimeAsync(progressIntervalMilliseconds);
    expect(sent).toEqual([10]);

    await advanceTimersByTimeAsync(progressIntervalMilliseconds - 1);
    expect(sent).toEqual([10]);
    failSends(false);
    await advanceTimersByTimeAsync(1);
    expect(sent).toEqual([10, 10]);
    await advanceTimersByTimeAsync(progressIntervalMilliseconds);

    expect(sent).toEqual([10, 10]);
  });

  it("does not queue failed positions: the next report carries only the latest", async () => {
    const { reporter, sent, failSends } = createHarness();
    failSends(true);
    reporter.notify("play");
    for (const position of [10, 20, 30]) {
      reporter.update(position);
      await advanceTimersByTimeAsync(progressIntervalMilliseconds);
    }
    failSends(false);
    reporter.update(40);
    await advanceTimersByTimeAsync(progressIntervalMilliseconds);

    expect(sent).toEqual([10, 20, 30, 40]);
    expect(timerCount()).toBe(1);
  });

  it("keeps a newer report when an older one fails late", async () => {
    let rejectFirst: (reason: Error) => void = () => undefined;
    const sent: number[] = [];
    const reporter = createProgressReporter({
      send: (positionSeconds) => {
        sent.push(positionSeconds);
        return sent.length === 1
          ? new Promise<void>((_resolve, reject) => {
              rejectFirst = reject;
            })
          : Promise.resolve();
      },
      clock: fakeTimerClock,
    });
    reporter.update(10);
    reporter.notify("seeked");
    advanceTimersByTime(progressThrottleMilliseconds);
    reporter.update(20);
    reporter.notify("seeked");
    rejectFirst(new Error("late"));
    await advanceTimersByTimeAsync(progressThrottleMilliseconds);
    reporter.notify("pause");

    expect(sent).toEqual([10, 20]);
  });

  it("survives a send that throws and sends the position again with the next report", () => {
    let calls = 0;
    const reporter = createProgressReporter({
      send: () => {
        calls += 1;
        throw new Error("no fetch");
      },
      clock: fakeTimerClock,
    });
    reporter.update(10);

    reporter.notify("pause");
    advanceTimersByTime(progressThrottleMilliseconds);
    reporter.notify("pause");

    expect(calls).toBe(2);
  });
});
