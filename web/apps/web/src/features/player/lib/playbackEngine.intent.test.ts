import { describe, expect, it, vi } from "vitest";

import { problemResponse } from "../../../test/fakeApi";
import { createEngineHarness, fatalHlsError, type EngineHarness } from "../test/engineHarness";
import { createdSession, sessionResponse } from "../test/fakePlaybackServer";

/** An hls.js stream that plays, paused by the user (task M2-41). */
async function pausedHls(): Promise<EngineHarness> {
  const harness = createEngineHarness({ delivery: "hls" });
  harness.open();
  await harness.untilSession("session-1");
  await vi.waitFor(() => {
    expect(harness.media.calls).toEqual(["play"]);
  });
  harness.media.emit("loadeddata");
  harness.engine.setAutoplay(false);
  return harness;
}

/** Waits until the `count`th hls.js instance has loaded its stream. */
async function untilStreamLoaded(harness: EngineHarness, count: number): Promise<void> {
  await vi.waitFor(() => {
    expect(harness.hls.log.at(-1)).toMatch(new RegExp(`^load ${String(count)} `));
  });
  // A play() would follow on a microtask.
  await Promise.resolve();
}

/** Expects that the element was not played since it was paused, then plays it again. */
async function expectPausedUntilPlay(harness: EngineHarness): Promise<void> {
  const { engine, media, snapshot } = harness;
  expect(media.calls.slice(media.calls.lastIndexOf("pause"))).not.toContain("play");
  expect(snapshot().autoplay).toBe(false);

  engine.setAutoplay(true);

  await vi.waitFor(() => {
    expect(media.calls.at(-1)).toBe("play");
  });
  expect(snapshot().autoplay).toBe(true);
}

describe("PlaybackEngine keeps a pause the user made while playing", () => {
  it("through a recovery after a fatal hls.js network error", async () => {
    const harness = await pausedHls();

    harness.hls.players[0]?.report(fatalHlsError("fragLoadError", 500));
    await harness.untilSession("session-2");
    await untilStreamLoaded(harness, 2);

    await expectPausedUntilPlay(harness);
  });

  it("through a retry of a stream that was not ready", async () => {
    const harness = await pausedHls();

    harness.hls.players[0]?.report(fatalHlsError("manifestLoadError", 503));
    harness.timers.pending()[0]?.fire();
    await untilStreamLoaded(harness, 2);

    expect(harness.server.sessionRequests).toHaveLength(1);
    await expectPausedUntilPlay(harness);
  });
});

describe("PlaybackEngine retry", () => {
  function failingFirst(): EngineHarness {
    return createEngineHarness({
      delivery: "hls",
      createSession: (request, index) =>
        index === 0
          ? problemResponse(503, "session_limit_reached")
          : sessionResponse(createdSession(request, index, "hls")),
    });
  }

  it("keeps a pause chosen in the error state", async () => {
    const harness = failingFirst();
    harness.open();
    await harness.until((current) => current.status === "error");
    harness.engine.setAutoplay(false);

    harness.engine.retry();

    expect(harness.snapshot().autoplay).toBe(false);
    await harness.untilSession("session-2");
    await untilStreamLoaded(harness, 1);
    expect(harness.media.calls).not.toContain("play");
  });

  it("plays with the default intent", async () => {
    const harness = failingFirst();
    harness.open();
    await harness.until((current) => current.status === "error");

    harness.engine.retry();

    await harness.untilSession("session-2");
    await vi.waitFor(() => {
      expect(harness.media.calls).toEqual(["play"]);
    });
    expect(harness.snapshot().autoplay).toBe(true);
  });
});
