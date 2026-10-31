import { describe, expect, it, vi } from "vitest";

import { jsonResponse, problemResponse } from "../../../test/fakeApi";
import { createEngineHarness, fatalHlsError, type EngineHarness } from "../test/engineHarness";
import { fakeSession, nativeHlsProfile } from "../test/playerFixtures";
import { maxStreamNotReadyRetries } from "./playbackEngine";
import { recoveryProgressSeconds } from "./recoveryBudget";

const mediaErrorAborted = 1;
const mediaErrorNetwork = 2;

const mediaError = {
  error: { type: "mediaError", details: "bufferAppendError", fatal: true },
  retryAfterSeconds: undefined,
};

async function playingHls(): Promise<EngineHarness> {
  const harness = createEngineHarness({ delivery: "hls" });
  harness.open();
  await harness.untilSession("session-1");
  await vi.waitFor(() => {
    expect(harness.hls.players).toHaveLength(1);
  });
  harness.media.emit("loadeddata");
  return harness;
}

describe("PlaybackEngine recovery from hls.js errors", () => {
  it("recreates the session once at the current position after a fatal segment error", async () => {
    const { media, hls, server, snapshot, untilSession, until } = await playingHls();
    media.playTo(42);

    hls.players[0]?.report(fatalHlsError("fragLoadError", 500));
    await untilSession("session-2");
    expect(server.sessionRequests[1]).toMatchObject({
      startPositionSeconds: 42,
      replacesSessionId: "session-1",
    });
    await vi.waitFor(() => {
      expect(hls.players).toHaveLength(2);
    });

    // The recovered session fails again at the same position: shown as an error, no loop.
    hls.players[1]?.report(fatalHlsError("fragLoadError", 500));
    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
    expect(server.sessionRequests).toHaveLength(2);
    expect(hls.log.at(-1)).toBe("destroy 2");
  });

  it("recovers again once playback has moved on", async () => {
    const { media, hls, server, untilSession } = await playingHls();
    hls.players[0]?.report(fatalHlsError("fragLoadTimeOut"));
    await untilSession("session-2");
    await vi.waitFor(() => {
      expect(hls.players).toHaveLength(2);
    });
    media.emit("loadeddata");

    media.playTo(recoveryProgressSeconds);
    hls.players[1]?.report(fatalHlsError("levelLoadError", 404));

    await untilSession("session-3");
    expect(server.sessionRequests[2]?.startPositionSeconds).toBe(recoveryProgressSeconds);
  });

  it("recovers again after the user picked a new position", async () => {
    const { engine, hls, server, untilSession } = await playingHls();
    hls.players[0]?.report(fatalHlsError("fragLoadError", 404));
    await untilSession("session-2");
    await vi.waitFor(() => {
      expect(hls.players).toHaveLength(2);
    });

    engine.seek(80);
    await untilSession("session-3");
    await vi.waitFor(() => {
      expect(hls.players).toHaveLength(3);
    });
    hls.players[2]?.report(fatalHlsError("fragLoadError", 404));

    await untilSession("session-4");
    expect(server.sessionRequests).toHaveLength(4);
  });

  it("ignores non-fatal errors, which hls.js retries itself", async () => {
    const { hls, server, snapshot } = await playingHls();

    hls.players[0]?.report({
      ...fatalHlsError("fragLoadError", 404),
      error: { type: "networkError", details: "fragLoadError", fatal: false },
    });

    expect(snapshot().status).toBe("playing");
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("lets hls.js recover a fatal media error once per position", async () => {
    const { engine, media, hls, server, snapshot } = await playingHls();
    media.playTo(20);

    hls.players[0]?.report(mediaError);
    expect(hls.log.at(-1)).toBe("recover 1");
    expect(snapshot().status).toBe("playing");

    // Playback moved on: another recovery is allowed.
    media.playTo(20 + recoveryProgressSeconds);
    hls.players[0]?.report(mediaError);
    expect(hls.log.at(-1)).toBe("recover 1");
    expect(hls.log.filter((entry) => entry === "recover 1")).toHaveLength(2);

    // Same position again: shown as an error.
    hls.players[0]?.report(mediaError);
    expect(snapshot()).toMatchObject({ status: "error", error: { code: "generic" } });
    expect(server.sessionRequests).toHaveLength(1);

    // A retry grants a new attempt.
    engine.retry();
    expect(snapshot().status).toBe("loading");
  });

  it("shows fatal errors that neither a new session nor hls.js can fix", async () => {
    const { hls, server, snapshot } = await playingHls();

    hls.players[0]?.report({
      error: { type: "otherError", details: "internalException", fatal: true },
      retryAfterSeconds: undefined,
    });

    expect(snapshot()).toMatchObject({ status: "error", error: { code: "generic" } });
    expect(hls.log).not.toContain("recover 1");
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("ignores errors of an hls.js instance that was already replaced", async () => {
    const { engine, hls, server, snapshot, untilSession } = await playingHls();
    engine.seek(100);
    await untilSession("session-2");

    hls.players[0]?.report(fatalHlsError("fragLoadError", 404));

    expect(snapshot().status).toBe("loading");
    expect(server.sessionRequests).toHaveLength(2);
  });
});

describe("PlaybackEngine recovery from media element errors", () => {
  it("looks the session up and recreates it when the server lost it", async () => {
    const { open, media, server, untilSession } = createEngineHarness({
      getSession: () => problemResponse(404, "not_found"),
    });
    open();
    await untilSession("session-1");
    media.emit("loadedmetadata");
    media.emit("loadeddata");
    media.playTo(25);

    media.fail(mediaErrorNetwork);

    await untilSession("session-2");
    expect(server.lookedUpSessions).toEqual(["session-1"]);
    expect(server.sessionRequests[1]).toMatchObject({
      startPositionSeconds: 25,
      replacesSessionId: "session-1",
    });
  });

  it.each([
    { name: "the session still plays", answer: () => jsonResponse(fakeSession({ id: "x" })) },
    { name: "the lookup fails", answer: () => problemResponse(500, "internal") },
  ])("shows an error when $name", async ({ answer }) => {
    const { open, media, server, snapshot, untilSession, until } = createEngineHarness({
      getSession: answer,
    });
    open();
    await untilSession("session-1");

    media.fail(mediaErrorNetwork);

    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("shows an error when the lookup fails without an HTTP status", async () => {
    const { open, media, snapshot, untilSession, until } = createEngineHarness({
      wrapApi: (api) => ({ ...api, getSession: () => Promise.reject(new Error("broken")) }),
    });
    open();
    await untilSession("session-1");

    media.fail(mediaErrorNetwork);

    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
  });

  it("waits for a native HLS stream that is still starting", async () => {
    const { open, media, timers, snapshot, untilSession, until } = createEngineHarness({
      profile: nativeHlsProfile,
      delivery: "hls",
      getSession: () => jsonResponse(fakeSession({ id: "session-1", state: "starting" })),
    });
    open();
    const session = await untilSession("session-1");

    media.fail(mediaErrorNetwork);
    await until((current) => current.waitingForStream);

    expect(media.src).toBe("");
    expect(timers.pending().map((timer) => timer.milliseconds)).toEqual([2_000]);
    timers.pending()[0]?.fire();
    expect(media.src).toBe(session.streamUrl);
    media.emit("loadeddata");
    expect(snapshot()).toMatchObject({ status: "playing", waitingForStream: false });
  });

  it("waits again when native HLS failed just before the first segment appeared", async () => {
    const { open, media, timers, untilSession, until } = createEngineHarness({
      profile: nativeHlsProfile,
      delivery: "hls",
      getSession: () => jsonResponse(fakeSession({ id: "session-1", state: "ready" })),
    });
    open();
    const session = await untilSession("session-1");

    media.fail(mediaErrorNetwork);
    await until((current) => current.waitingForStream);
    timers.pending()[0]?.fire();

    expect(media.src).toBe(session.streamUrl);
  });

  it.each([
    { state: "starting", code: "stream_not_ready" },
    // The session is ready, so the server is not the one still preparing: something else broke.
    { state: "ready", code: "generic" },
  ] as const)(
    "ends with $code when native HLS never plays while its session is $state",
    async ({ state, code }) => {
      const { open, media, timers, snapshot, untilSession, until } = createEngineHarness({
        profile: nativeHlsProfile,
        delivery: "hls",
        getSession: () => jsonResponse(fakeSession({ id: "session-1", state })),
      });
      open();
      await untilSession("session-1");

      for (let attempt = 0; attempt < maxStreamNotReadyRetries; attempt += 1) {
        media.fail(mediaErrorNetwork);
        await vi.waitFor(() => {
          expect(timers.pending()).toHaveLength(1);
        });
        timers.pending()[0]?.fire();
      }
      media.fail(mediaErrorNetwork);

      await until((current) => current.status === "error");
      expect(snapshot()).toMatchObject({ waitingForStream: false, error: { code } });
    },
  );

  it("shows an error when a playing native HLS stream breaks while its session is ready", async () => {
    const { open, media, snapshot, untilSession, until } = createEngineHarness({
      profile: nativeHlsProfile,
      delivery: "hls",
      getSession: () => jsonResponse(fakeSession({ id: "session-1", state: "ready" })),
    });
    open();
    await untilSession("session-1");
    media.emit("loadeddata");

    media.fail(mediaErrorNetwork);

    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
  });

  it("ignores a media error before any stream is attached", async () => {
    const { open, media, server, snapshot, untilSession } = createEngineHarness();
    open();

    media.fail(mediaErrorNetwork);
    await untilSession("session-1");

    expect(server.lookedUpSessions).toEqual([]);
    expect(snapshot().status).toBe("loading");
  });

  it("ignores aborted fetches and errors while hls.js owns the element", async () => {
    const direct = createEngineHarness();
    direct.open();
    await direct.untilSession("session-1");
    direct.media.fail(mediaErrorAborted);

    const viaHls = await playingHls();
    viaHls.media.fail(mediaErrorNetwork);
    await vi.waitFor(() => Promise.resolve());

    expect(direct.server.lookedUpSessions).toEqual([]);
    expect(viaHls.server.lookedUpSessions).toEqual([]);
  });

  it.each([
    { name: "lost session", answer: () => problemResponse(404, "not_found") },
    {
      name: "starting session",
      answer: () => jsonResponse(fakeSession({ id: "session-1", state: "starting" })),
    },
  ])("drops the lookup answer ($name) when the user moved on meanwhile", async ({ answer }) => {
    const heldLookups: ((response: Response) => void)[] = [];
    const { engine, open, media, server, timers, snapshot, untilSession } = createEngineHarness({
      getSession: () =>
        new Promise<Response>((resolve) => {
          heldLookups.push(resolve);
        }),
    });
    open();
    await untilSession("session-1");
    media.fail(mediaErrorNetwork);
    await vi.waitFor(() => {
      expect(heldLookups).toHaveLength(1);
    });

    engine.seek(50);
    await untilSession("session-2");
    heldLookups[0]?.(answer());
    await vi.waitFor(() => {
      expect(server.lookedUpSessions).toEqual(["session-1"]);
    });
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(server.sessionRequests).toHaveLength(2);
    expect(timers.pending()).toEqual([]);
    expect(snapshot()).toMatchObject({ status: "loading", waitingForStream: false });
  });
});
