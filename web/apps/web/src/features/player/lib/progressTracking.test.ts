import type { ProgressReporter, ProgressTrigger } from "@mivo/player-core";
import { describe, expect, it, vi } from "vitest";

import { createEngineHarness, type EngineHarness } from "../test/engineHarness";
import { trackProgress } from "./progressTracking";

interface Notification {
  readonly trigger: ProgressTrigger;
  /** The reporter's position when it was notified; `undefined` before the first update. */
  readonly position: number | undefined;
}

interface RecordingReporter extends ProgressReporter {
  readonly notifications: Notification[];
  readonly position: () => number | undefined;
  readonly disposed: () => boolean;
}

function createRecordingReporter(): RecordingReporter {
  const notifications: Notification[] = [];
  let position: number | undefined;
  let disposed = false;
  return {
    notifications,
    position: () => position,
    disposed: () => disposed,
    update: (positionSeconds) => {
      position = positionSeconds;
    },
    notify: (trigger) => {
      notifications.push({ trigger, position });
    },
    dispose: () => {
      disposed = true;
    },
  };
}

function track(harness: EngineHarness, offsetSeconds = 0) {
  const reporter = createRecordingReporter();
  const page = new EventTarget();
  const stop = trackProgress({
    source: harness.engine,
    media: harness.media,
    page,
    reporter,
    offsetSeconds,
  });
  return { reporter, page, stop };
}

function mediaEvent(harness: EngineHarness, type: string): void {
  harness.media.dispatchEvent(new Event(type));
}

async function playHls(harness: EngineHarness, startPositionSeconds = 0): Promise<void> {
  harness.open(startPositionSeconds);
  await harness.untilSession("session-1");
  harness.media.emit("loadeddata");
  await vi.waitFor(() => {
    expect(harness.media.calls).toContain("play");
  });
}

describe("trackProgress", () => {
  it("follows the position and reports media events with the element's time", async () => {
    const harness = createEngineHarness();
    const { reporter } = track(harness);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");

    mediaEvent(harness, "play");
    harness.media.playTo(12);
    expect(reporter.position()).toBe(12);
    // The element's clock is ahead of the last timeupdate, as in a browser.
    harness.media.currentTime = 12.2;
    mediaEvent(harness, "pause");
    harness.media.currentTime = 120;
    mediaEvent(harness, "ended");

    expect(reporter.notifications).toEqual([
      { trigger: "play", position: 0 },
      { trigger: "pause", position: 12.2 },
      { trigger: "ended", position: 120 },
    ]);
  });

  it("reports the end of a local seek, whichever listener hears it first", async () => {
    const harness = createEngineHarness();
    const { reporter } = track(harness);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");

    harness.media.currentTime = 90;
    mediaEvent(harness, "seeked");

    expect(reporter.notifications).toEqual([{ trigger: "seeked", position: 90 }]);
  });

  it("adds the session's start position to the media time (HLS)", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    const { reporter } = track(harness);
    await playHls(harness, 60);

    harness.media.playTo(5);
    mediaEvent(harness, "pause");

    expect(reporter.notifications).toEqual([{ trigger: "pause", position: 65 }]);
  });

  it("keeps the position through a remux seek restart and uses the new offset after it", async () => {
    const harness = createEngineHarness({ delivery: "hls" });
    const { reporter } = track(harness);
    await playHls(harness);
    harness.media.playTo(8);
    harness.media.bufferedRanges = [{ start: 0, end: 12 }];

    harness.engine.seek(100);
    // The element is empty while the new session is created; its clock reads 0.
    harness.media.currentTime = 0;
    mediaEvent(harness, "pause");
    expect(reporter.notifications).toEqual([{ trigger: "pause", position: 100 }]);

    await harness.untilSession("session-2");
    harness.media.emit("loadeddata");
    expect(reporter.notifications.at(-1)).toEqual({ trigger: "seeked", position: 100 });
    harness.media.playTo(5);
    mediaEvent(harness, "pause");

    expect(reporter.notifications.at(-1)).toEqual({ trigger: "pause", position: 105 });
  });

  it("does not report the start of playback as a seek", async () => {
    const harness = createEngineHarness();
    const { reporter } = track(harness);
    harness.open(30);
    await harness.untilSession("session-1");
    harness.media.emit("loadedmetadata");
    harness.media.emit("loadeddata");

    expect(reporter.notifications).toEqual([]);
    expect(reporter.position()).toBe(30);
  });

  it("adds the offset of an audiobook part", async () => {
    const harness = createEngineHarness();
    const { reporter } = track(harness, 3_600);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");

    harness.media.playTo(10);
    mediaEvent(harness, "pause");

    expect(reporter.notifications).toEqual([{ trigger: "pause", position: 3_610 }]);
  });

  it("reports the last position when the page is hidden, also after the engine let go", async () => {
    const harness = createEngineHarness();
    const { reporter, page } = track(harness);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");
    harness.media.playTo(42);

    harness.engine.suspend();
    harness.media.currentTime = 0;
    page.dispatchEvent(new Event("pagehide"));

    expect(reporter.notifications).toEqual([{ trigger: "pagehide", position: 42 }]);
  });

  it("starts the interval when the element already plays", async () => {
    const harness = createEngineHarness();
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");
    harness.media.playTo(3);
    harness.media.paused = false;

    const { reporter } = track(harness);

    expect(reporter.notifications).toEqual([{ trigger: "play", position: 3 }]);
  });

  it("reports stop with the current position and then listens to nothing", async () => {
    const harness = createEngineHarness();
    const { reporter, page, stop } = track(harness);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");
    harness.media.playTo(20);
    harness.media.currentTime = 21;

    stop();
    harness.media.playTo(50);
    mediaEvent(harness, "pause");
    page.dispatchEvent(new Event("pagehide"));

    expect(reporter.notifications).toEqual([{ trigger: "stop", position: 21 }]);
    expect(reporter.position()).toBe(21);
    expect(reporter.disposed()).toBe(true);
  });

  it("keeps the last position when the engine closed before the tracking stops", async () => {
    const harness = createEngineHarness();
    const { reporter, stop } = track(harness);
    harness.open();
    await harness.untilSession("session-1");
    harness.media.emit("loadeddata");
    harness.media.playTo(20);

    harness.engine.close();
    stop();

    expect(reporter.notifications).toEqual([{ trigger: "stop", position: 20 }]);
  });
});
