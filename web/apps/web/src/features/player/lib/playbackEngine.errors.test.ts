import { describe, expect, it, vi } from "vitest";

import { problemResponse } from "../../../test/fakeApi";
import { createEngineHarness, fatalHlsError, type EngineHarness } from "../test/engineHarness";
import { createdSession, problemWithRetryAfter, sessionResponse } from "../test/fakePlaybackServer";
import { maxStreamNotReadyRetries } from "./playbackEngine";

async function attachedHls(): Promise<EngineHarness> {
  const harness = createEngineHarness({ delivery: "hls" });
  harness.open();
  await harness.untilSession("session-1");
  await vi.waitFor(() => {
    expect(harness.hls.players).toHaveLength(1);
  });
  return harness;
}

function streamNotReady(retryAfterSeconds?: number) {
  return fatalHlsError("manifestLoadError", 503, retryAfterSeconds);
}

describe("PlaybackEngine stream_not_ready", () => {
  it("waits the Retry-After delay and loads the same stream again", async () => {
    const { media, hls, timers, server, snapshot } = await attachedHls();

    hls.players[0]?.report(streamNotReady(5));

    expect(snapshot()).toMatchObject({ status: "loading", waitingForStream: true });
    expect(hls.log.at(-1)).toBe("destroy 1");
    expect(timers.pending().map((timer) => timer.milliseconds)).toEqual([5_000]);
    timers.pending()[0]?.fire();
    await vi.waitFor(() => {
      expect(hls.log.slice(-2)).toEqual([
        "attach 2",
        "load 2 /api/v1/playback/sessions/session-1/hls/index.m3u8",
      ]);
    });
    media.emit("loadeddata");
    expect(snapshot()).toMatchObject({ status: "playing", waitingForStream: false });
    expect(server.sessionRequests).toHaveLength(1);
  });

  it.each([
    { name: "without Retry-After", retryAfter: undefined, delay: 2_000 },
    { name: "with a very long Retry-After", retryAfter: 3_600, delay: 30_000 },
  ])("waits a bounded default delay $name", async ({ retryAfter, delay }) => {
    const { hls, timers } = await attachedHls();

    hls.players[0]?.report(streamNotReady(retryAfter));

    expect(timers.pending().map((timer) => timer.milliseconds)).toEqual([delay]);
  });

  it("gives up with stream_not_ready after the last attempt", async () => {
    const { hls, timers, snapshot } = await attachedHls();

    for (let attempt = 0; attempt < maxStreamNotReadyRetries; attempt += 1) {
      hls.players[attempt]?.report(streamNotReady());
      timers.pending()[0]?.fire();
      await vi.waitFor(() => {
        expect(hls.players).toHaveLength(attempt + 2);
      });
    }
    hls.players.at(-1)?.report(streamNotReady());

    expect(snapshot()).toMatchObject({
      status: "error",
      waitingForStream: false,
      error: { code: "stream_not_ready" },
    });
    expect(timers.pending()).toEqual([]);
  });

  it("cancels the wait when the user seeks", async () => {
    const { engine, hls, timers, untilSession } = await attachedHls();
    hls.players[0]?.report(streamNotReady());

    engine.seek(30);

    expect(timers.pending()).toEqual([]);
    await untilSession("session-2");
  });
});

describe("PlaybackEngine session errors", () => {
  it.each([
    { status: 422, code: "no_compatible_format" },
    { status: 422, code: "transcode_unavailable" },
  ])("reports $code from the server", async ({ status, code }) => {
    const { open, snapshot, until } = createEngineHarness({
      createSession: () => problemResponse(status, code),
    });

    open();

    await until((current) => current.status === "error");
    expect(snapshot()).toMatchObject({ error: { code }, session: undefined });
  });

  it("reports session_limit_reached with Retry-After and succeeds on retry", async () => {
    const { engine, open, server, snapshot, until, untilSession } = createEngineHarness({
      createSession: (request, index) =>
        index === 0
          ? problemWithRetryAfter(503, "session_limit_reached", "7")
          : sessionResponse(createdSession(request, index)),
    });
    open(15);
    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "session_limit_reached", retryAfterSeconds: 7 });

    engine.retry();

    await untilSession("session-2");
    expect(server.sessionRequests[1]).toMatchObject({ startPositionSeconds: 15 });
    expect(server.sessionRequests[1]?.replacesSessionId).toBeUndefined();
  });

  it.each([
    { name: "an invalid request", respond: () => problemResponse(400, "invalid") },
    { name: "a code of a later server", respond: () => problemResponse(422, "unsupported") },
    { name: "a failed connection", respond: () => Promise.reject(new TypeError("offline")) },
  ])("reports $name as a generic error", async ({ respond }) => {
    const { open, snapshot, until } = createEngineHarness({ createSession: respond });

    open();

    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
  });

  it("restarts at the current position with the old session replaced after a failed seek", async () => {
    const { engine, open, media, server, snapshot, until, untilSession } = createEngineHarness({
      delivery: "hls",
      createSession: (request, index) =>
        index === 1
          ? problemResponse(500, "internal")
          : sessionResponse(createdSession(request, index, "hls")),
    });
    open();
    await untilSession("session-1");
    media.emit("loadeddata");
    engine.seek(70);
    await until((current) => current.status === "error");
    expect(snapshot().session?.id).toBe("session-1");

    engine.retry();

    await untilSession("session-3");
    expect(server.sessionRequests[2]).toMatchObject({
      startPositionSeconds: 70,
      replacesSessionId: "session-1",
    });
  });
});
