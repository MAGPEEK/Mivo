import { describe, expect, it, vi } from "vitest";

import { jsonResponse } from "../../../test/fakeApi";
import { createEngineHarness } from "../test/engineHarness";
import { sessionResponse } from "../test/fakePlaybackServer";
import {
  fakeSession,
  mseProfile,
  nativeHlsProfile,
  noHlsProfile,
  playerFileId,
} from "../test/playerFixtures";

describe("PlaybackEngine delivery", () => {
  it("requests a session with the profile and plays a direct-play file through src", async () => {
    const { open, server, media, hls, snapshot, untilSession } = createEngineHarness();

    open();
    expect(snapshot().status).toBe("loading");
    const session = await untilSession("session-1");

    expect(server.sessionRequests).toEqual([{ fileId: playerFileId, profile: mseProfile }]);
    expect(media.src).toBe(session.streamUrl);
    expect(hls.log).toEqual([]);
    await vi.waitFor(() => {
      expect(media.calls).toContain("play");
    });
    media.emit("loadeddata");
    expect(snapshot()).toMatchObject({ status: "playing", session, position: 0 });
  });

  it("starts a direct-play file at the requested position once its metadata is known", async () => {
    const { open, server, media, snapshot, untilSession } = createEngineHarness();

    open(30);
    await untilSession("session-1");

    expect(server.sessionRequests[0]?.startPositionSeconds).toBe(30);
    expect(snapshot().position).toBe(30);
    // Before the jump the file's clock reads 0; that must not move the displayed position.
    media.emit("timeupdate");
    expect(snapshot().position).toBe(30);
    media.emit("loadedmetadata");
    expect(media.currentTime).toBe(30);
    media.playTo(31.5);
    expect(snapshot().position).toBe(31.5);
    // A second metadata event (e.g. a new resolution) keeps the position.
    media.emit("loadedmetadata");
    expect(media.currentTime).toBe(31.5);
  });

  it("plays an HLS session through hls.js when the profile uses MSE", async () => {
    const { open, media, hls, snapshot, untilSession } = createEngineHarness({ delivery: "hls" });

    open();
    const session = await untilSession("session-1");
    await vi.waitFor(() => {
      expect(hls.log).toEqual(["create 1", "attach 1", `load 1 ${session.streamUrl}`]);
    });

    expect(media.src).toBe("");
    media.emit("loadeddata");
    expect(snapshot().status).toBe("playing");
  });

  it("shows the position of an HLS stream relative to its actual start", async () => {
    const { open, media, snapshot, untilSession } = createEngineHarness({
      createSession: () =>
        sessionResponse(fakeSession({ id: "s", delivery: "hls", startPositionSeconds: 58.5 })),
    });

    open(60);
    await untilSession("s");
    expect(snapshot().position).toBe(58.5);

    media.playTo(5);
    expect(snapshot().position).toBe(63.5);
    const unchanged = snapshot();
    media.emit("seeked");
    media.emit("seeking");
    expect(snapshot()).toBe(unchanged);
  });

  it("plays an HLS session with the media element when the profile uses native HLS", async () => {
    const { open, media, hls, untilSession } = createEngineHarness({
      delivery: "hls",
      profile: nativeHlsProfile,
    });

    open();
    const session = await untilSession("session-1");

    expect(media.src).toBe(session.streamUrl);
    expect(hls.log).toEqual([]);
  });

  it.each([
    { name: "an HLS session for a profile without HLS", profile: noHlsProfile, delivery: "hls" },
    { name: "a delivery this version does not know", profile: mseProfile, delivery: "dash" },
  ])("refuses $name", async ({ profile, delivery }) => {
    const { open, media, hls, snapshot, untilSession } = createEngineHarness({
      profile,
      // Raw JSON: the typed fixture cannot hold a delivery the API version does not define.
      createSession: () => jsonResponse({ ...fakeSession({ id: "s" }), delivery }, 201),
    });

    open();
    await untilSession("s");

    expect(snapshot()).toMatchObject({ status: "error", error: { code: "generic" } });
    expect(media.src).toBe("");
    expect(hls.log).toEqual([]);
  });

  it("fails generically when hls.js cannot be loaded", async () => {
    const { open, snapshot, until } = createEngineHarness({
      delivery: "hls",
      hlsFailsToLoad: true,
    });

    open();

    await until((current) => current.status === "error");
    expect(snapshot().error).toEqual({ code: "generic" });
  });

  it("stays quiet when the browser refuses to autoplay", async () => {
    const { open, media, snapshot, untilSession } = createEngineHarness();
    media.playResult = () => Promise.reject(new DOMException("No gesture", "NotAllowedError"));

    open();
    await untilSession("session-1");
    await vi.waitFor(() => {
      expect(media.calls).toContain("play");
    });

    expect(snapshot().status).toBe("loading");
  });

  it("ignores media events before a stream is attached", () => {
    const { open, media, snapshot } = createEngineHarness();

    open(10);
    media.emit("loadeddata");
    media.emit("loadedmetadata");
    media.playTo(3);

    expect(snapshot()).toMatchObject({ status: "loading", position: 10 });
    expect(media.currentTime).toBe(3);
  });

  it("notifies subscribers until they unsubscribe", async () => {
    const { engine, open, untilSession } = createEngineHarness();
    const listener = vi.fn();
    const unsubscribe = engine.subscribe(listener);

    open();
    await untilSession("session-1");
    const calls = listener.mock.calls.length;
    unsubscribe();
    engine.close();

    expect(calls).toBeGreaterThan(0);
    expect(listener).toHaveBeenCalledTimes(calls);
  });
});
