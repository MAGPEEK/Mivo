import type { PlaybackRequest } from "@mivo/api-client";
import { describe, expect, it, vi } from "vitest";

import { problemResponse } from "../../../test/fakeApi";
import { createEngineHarness } from "../test/engineHarness";
import { createdSession, sessionResponse } from "../test/fakePlaybackServer";
import { fakeSession, playerFileDurationSeconds } from "../test/playerFixtures";

/** A session request the test answers when it wants to. */
interface HeldAnswer {
  readonly release: () => void;
  readonly fail: () => void;
}

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
    const withoutDuration = fakeSession({ id: "s" });
    delete withoutDuration.durationSeconds;
    const { engine, open, server, untilSession } = createEngineHarness({
      createSession: (request, index) =>
        index === 0
          ? sessionResponse(withoutDuration)
          : sessionResponse(createdSession(request, index)),
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
});

/** Session requests the test answers one by one, counting how many were in flight at once. */
function createHeldAnswers(delivery: "file" | "hls") {
  const held: HeldAnswer[] = [];
  let inFlight = 0;
  let maxInFlight = 0;
  const respond = (request: PlaybackRequest, index: number) =>
    new Promise<Response>((resolve) => {
      inFlight += 1;
      maxInFlight = Math.max(maxInFlight, inFlight);
      const settle = (response: Response) => {
        inFlight -= 1;
        resolve(response);
      };
      held[index] = {
        release: () => {
          settle(sessionResponse(createdSession(request, index, delivery)));
        },
        fail: () => {
          settle(problemResponse(500, "internal"));
        },
      };
    });
  const answer = async (index: number, how: "release" | "fail") => {
    await vi.waitFor(() => {
      expect(held[index]).toBeDefined();
    });
    held[index]?.[how]();
  };
  return {
    respond,
    release: (index: number) => answer(index, "release"),
    fail: (index: number) => answer(index, "fail"),
    maxInFlight: () => maxInFlight,
  };
}
