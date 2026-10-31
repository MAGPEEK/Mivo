import { describe, expect, it, vi } from "vitest";

import { createEngineHarness } from "../test/engineHarness";
import { createdSession, sessionResponse } from "../test/fakePlaybackServer";
import { createHeldAnswers } from "../test/heldAnswers";
import { fakeSession, mseProfile, playerFileDurationSeconds } from "../test/playerFixtures";

describe("PlaybackEngine seek", () => {
  it("seeks a direct-play file locally", async () => {
    const { engine, open, server, media, snapshot, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");
    media.emit("loadeddata");

    engine.seek(90);

    expect(media.currentTime).toBe(90);
    expect(snapshot().position).toBe(90);
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("seeks an HLS stream locally inside the produced range", async () => {
    const { engine, open, server, media, snapshot, untilSession } = createEngineHarness({
      createSession: () =>
        sessionResponse(fakeSession({ id: "s", delivery: "hls", startPositionSeconds: 60 })),
    });
    open(60);
    await untilSession("s");
    media.emit("loadeddata");
    await vi.waitFor(() => {
      expect(media.calls).toContain("play");
    });
    media.seekableRanges = [{ start: 0, end: 30 }];

    engine.seek(70);

    expect(media.currentTime).toBe(10);
    expect(snapshot().position).toBe(70);
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("restarts outside the produced range and destroys the old hls.js first", async () => {
    const { engine, open, server, media, hls, snapshot, untilSession } = createEngineHarness({
      delivery: "hls",
    });
    open();
    await untilSession("session-1");
    media.emit("loadeddata");
    await vi.waitFor(() => {
      expect(media.calls).toContain("play");
    });
    media.bufferedRanges = [{ start: 0, end: 12 }];

    engine.seek(100);

    expect(snapshot()).toMatchObject({ status: "loading", position: 100 });
    const session = await untilSession("session-2");
    expect(server.sessionRequests[1]).toMatchObject({
      startPositionSeconds: 100,
      replacesSessionId: "session-1",
    });
    await vi.waitFor(() => {
      expect(hls.log).toEqual([
        "create 1",
        "attach 1",
        "load 1 /api/v1/playback/sessions/session-1/hls/index.m3u8",
        "destroy 1",
        "create 2",
        "attach 2",
        `load 2 ${session.streamUrl}`,
      ]);
    });
    // The server replaces the old session itself; the client does not stop it again.
    expect(server.stoppedSessions).toEqual([]);
  });

  it("keeps a paused player paused across a restart", async () => {
    const { engine, open, media, hls, untilSession } = createEngineHarness({ delivery: "hls" });
    open();
    await untilSession("session-1");
    media.emit("loadeddata");
    await vi.waitFor(() => {
      expect(media.calls).toEqual(["play"]);
    });
    media.paused = true;

    engine.seek(100);
    await untilSession("session-2");
    await vi.waitFor(() => {
      expect(hls.log).toContain("attach 2");
    });

    expect(media.calls).toEqual(["play"]);
  });

  it("restarts at most one second before the end of the file", async () => {
    const { engine, open, server, media, untilSession } = createEngineHarness({ delivery: "hls" });
    open();
    await untilSession("session-1");

    // Not playing yet: every seek restarts, also one past the end.
    engine.seek(10_000);
    await untilSession("session-2");
    media.emit("loadeddata");
    engine.retry();
    await untilSession("session-3");

    const lastStart = playerFileDurationSeconds - 1;
    expect(server.sessionRequests.map((request) => request.startPositionSeconds)).toEqual([
      undefined,
      lastStart,
      lastStart,
    ]);
  });

  it("restarts anywhere after the start when the duration is unknown", async () => {
    const withoutDuration = fakeSession({ id: "s", delivery: "hls" });
    delete withoutDuration.durationSeconds;
    const { engine, open, server, untilSession } = createEngineHarness({
      createSession: (request, index) =>
        index === 0
          ? sessionResponse(withoutDuration)
          : sessionResponse(createdSession(request, index, "hls")),
    });
    open();
    await untilSession("s");

    engine.seek(10_000);

    await untilSession("session-2");
    expect(server.sessionRequests[1]?.startPositionSeconds).toBe(10_000);
  });

  it("ignores a seek before anything was opened", () => {
    const { engine, server, snapshot } = createEngineHarness();

    engine.seek(10);

    expect(snapshot().position).toBe(0);
    expect(server.requests).toHaveLength(0);
  });
});

describe("PlaybackEngine seek while a session request is in flight", () => {
  it("sends one request at a time and lets the newest target replace the session that arrived", async () => {
    const answers = createHeldAnswers("hls");
    const { engine, open, server, media, hls, snapshot, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await answers.release(0);
    await untilSession("session-1");
    media.emit("loadeddata");

    engine.seek(50);
    engine.seek(60);
    engine.seek(70);
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });
    expect(snapshot()).toMatchObject({ status: "loading", position: 70 });
    expect(server.sessionRequests[1]).toMatchObject({
      startPositionSeconds: 50,
      replacesSessionId: "session-1",
    });

    await answers.release(1);
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(3);
    });
    // The follow-up reuses the slot of the session that just arrived; 60 was overtaken.
    expect(server.sessionRequests[2]).toMatchObject({
      startPositionSeconds: 70,
      replacesSessionId: "session-2",
    });
    // The overlay describes the session that exists now, not the replaced one.
    expect(snapshot().session?.id).toBe("session-2");
    await answers.release(2);
    await untilSession("session-3");

    expect(answers.maxInFlight()).toBe(1);
    expect(server.stoppedSessions).toEqual([]);
    await vi.waitFor(() => {
      expect(hls.log.filter((entry) => entry.startsWith("attach"))).toEqual([
        "attach 1",
        "attach 2",
      ]);
    });
    expect(hls.log.at(-1)).toBe("load 2 /api/v1/playback/sessions/session-3/hls/index.m3u8");
  });

  it("applies a seek made before the first HLS session arrived with a replacing request", async () => {
    const answers = createHeldAnswers("hls");
    const { engine, open, server, snapshot } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.seek(40);
    expect(snapshot().position).toBe(40);
    await answers.release(0);

    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });
    expect(server.sessionRequests[1]).toMatchObject({
      startPositionSeconds: 40,
      replacesSessionId: "session-1",
    });
  });

  it("seeks inside a direct-play file that arrives after the seek, without a new session", async () => {
    const answers = createHeldAnswers("file");
    const { engine, open, server, media, snapshot } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.seek(10_000);
    await answers.release(0);

    await vi.waitFor(() => {
      expect(media.src).toBe("/api/v1/playback/sessions/session-1/file");
    });
    expect(server.sessionRequests).toHaveLength(1);
    expect(snapshot().position).toBe(playerFileDurationSeconds - 1);
    media.emit("loadedmetadata");
    expect(media.currentTime).toBe(playerFileDurationSeconds - 1);
  });

  it("sends the newest target when the request in flight fails", async () => {
    const answers = createHeldAnswers("hls");
    const { engine, open, server, media, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await answers.release(0);
    await untilSession("session-1");
    media.emit("loadeddata");

    engine.seek(50);
    engine.seek(60);
    await answers.fail(1);

    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(3);
    });
    expect(server.sessionRequests[2]).toMatchObject({
      startPositionSeconds: 60,
      replacesSessionId: "session-1",
    });
  });

  it("drops a waiting target on close and stops the session that arrives afterwards", async () => {
    const answers = createHeldAnswers("hls");
    const { engine, open, server, media, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await answers.release(0);
    await untilSession("session-1");
    media.emit("loadeddata");
    engine.seek(50);
    engine.seek(60);
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });

    engine.close();
    await answers.release(1);

    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1", "session-2"]);
    });
    expect(server.sessionRequests).toHaveLength(2);
  });

  it("opens another file without clamping its start to the previous file's late session", async () => {
    const answers = createHeldAnswers("file");
    const { engine, open, server, media, snapshot, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });
    const longerStart = playerFileDurationSeconds * 4;

    engine.open({
      media,
      profile: mseProfile,
      fileId: "file-2",
      startPositionSeconds: longerStart,
    });
    await answers.release(0);

    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });
    // The late session of the first file gives its slot to the new file and is never shown.
    expect(server.sessionRequests[1]).toMatchObject({
      fileId: "file-2",
      startPositionSeconds: longerStart,
      replacesSessionId: "session-1",
    });
    expect(snapshot().session).toBeUndefined();
    expect(media.src).toBe("");
    await answers.release(1);
    await untilSession("session-2");
    expect(media.src).toBe("/api/v1/playback/sessions/session-2/file");
    expect(answers.maxInFlight()).toBe(1);
  });
});

describe("PlaybackEngine seek on a direct-play file that has not loaded yet", () => {
  it("moves the pending start instead of creating a new session", async () => {
    const { engine, open, server, media, snapshot, untilSession } = createEngineHarness();
    open(30);
    await untilSession("session-1");

    engine.seek(50);

    expect(snapshot()).toMatchObject({ status: "loading", position: 50 });
    media.emit("loadedmetadata");
    expect(media.currentTime).toBe(50);
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("seeks the element when the file starts at its beginning", async () => {
    const { engine, open, server, media, snapshot, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");

    engine.seek(50);

    expect(media.currentTime).toBe(50);
    expect(snapshot().position).toBe(50);
    expect(server.sessionRequests).toHaveLength(1);
  });
});

describe("PlaybackEngine bounds after opening another file during a request", () => {
  async function openSecondFile() {
    const answers = createHeldAnswers("hls");
    const harness = createEngineHarness({ createSession: answers.respond });
    harness.open();
    await vi.waitFor(() => {
      expect(harness.server.sessionRequests).toHaveLength(1);
    });
    const start = playerFileDurationSeconds * 4;
    harness.engine.open({
      media: harness.media,
      profile: mseProfile,
      fileId: "file-2",
      startPositionSeconds: start,
    });
    await answers.release(0);
    await vi.waitFor(() => {
      expect(harness.server.sessionRequests).toHaveLength(2);
    });
    return { ...harness, answers, start };
  }

  it("does not clamp a seek during the new file's request to the old file's duration", async () => {
    const { engine, snapshot, start } = await openSecondFile();

    engine.seek(start - 10);

    expect(snapshot().position).toBe(start - 10);
  });

  it("does not clamp a retry after the new file's request failed", async () => {
    const { engine, server, answers, until, start } = await openSecondFile();
    await answers.fail(1);
    await until((current) => current.status === "error");

    engine.retry();

    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(3);
    });
    expect(server.sessionRequests[2]).toMatchObject({
      fileId: "file-2",
      startPositionSeconds: start,
      replacesSessionId: "session-1",
    });
  });
});

describe("PlaybackEngine play intent", () => {
  it("keeps a stream that arrives after a pause request paused", async () => {
    const answers = createHeldAnswers("file");
    const { engine, open, media, snapshot, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();

    engine.setAutoplay(false);
    expect(snapshot().autoplay).toBe(false);
    await answers.release(0);
    await untilSession("session-1");
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(media.calls).not.toContain("play");
  });

  it("plays and pauses an attached stream that is still loading", async () => {
    const { engine, open, media, snapshot, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");
    await vi.waitFor(() => {
      expect(media.calls).toEqual(["play"]);
    });

    engine.setAutoplay(false);
    engine.setAutoplay(true);

    await vi.waitFor(() => {
      expect(media.calls).toEqual(["play", "pause", "play"]);
    });
    expect(snapshot().autoplay).toBe(true);
  });

  it("plays every newly opened file, whatever the intent was before", async () => {
    const { engine, open, snapshot, untilSession } = createEngineHarness();
    engine.setAutoplay(false);
    expect(snapshot().autoplay).toBe(true);
    open();
    await untilSession("session-1");
    engine.setAutoplay(false);

    open();

    expect(snapshot().autoplay).toBe(true);
  });
});
