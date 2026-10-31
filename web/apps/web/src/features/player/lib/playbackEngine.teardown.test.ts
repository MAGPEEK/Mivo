import { describe, expect, it, vi } from "vitest";

import { problemResponse } from "../../../test/fakeApi";
import { createEngineHarness } from "../test/engineHarness";
import type { FakeMedia } from "../test/fakeMedia";
import { createdSession, sessionResponse } from "../test/fakePlaybackServer";
import { createHeldAnswers } from "../test/heldAnswers";
import type { HlsPlayer } from "./hlsPlayer";
import { initialPlaybackSnapshot } from "./playbackEngine";

describe("PlaybackEngine teardown", () => {
  it("destroys hls.js and stops the session with a keepalive request on close", async () => {
    const { engine, open, media, hls, server, snapshot, untilSession } = createEngineHarness({
      delivery: "hls",
    });
    open();
    await untilSession("session-1");
    await vi.waitFor(() => {
      expect(hls.log).toContain("attach 1");
    });

    engine.close();

    expect(hls.log.at(-1)).toBe("destroy 1");
    expect(snapshot()).toBe(initialPlaybackSnapshot);
    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(server.stopRequests[0]?.keepalive).toBe(true);
    // Listeners are gone: the element's events no longer reach the engine.
    media.emit("loadeddata");
    media.playTo(12);
    expect(snapshot()).toBe(initialPlaybackSnapshot);
  });

  it("empties the media element of a direct-play session on close", async () => {
    const { engine, open, media, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");

    engine.close();

    expect(media.src).toBe("");
    expect(media.calls.slice(-2)).toEqual(["removeAttribute src", "load"]);
  });

  it("stops a session that arrives after close", async () => {
    let release: () => void = () => undefined;
    const { engine, open, media, server } = createEngineHarness({
      createSession: (request, index) =>
        new Promise<Response>((resolve) => {
          release = () => {
            resolve(sessionResponse(createdSession(request, index)));
          };
        }),
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.close();
    release();

    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(media.src).toBe("");
  });

  it("destroys an hls.js instance that loads after close", async () => {
    const loads: ((player: HlsPlayer<FakeMedia>) => void)[] = [];
    const player = {
      attachMedia: vi.fn(),
      loadSource: vi.fn(),
      recoverMediaError: vi.fn(),
      destroy: vi.fn(),
    };
    const { engine, open, server, untilSession } = createEngineHarness({
      delivery: "hls",
      createHls: () =>
        new Promise((resolve) => {
          loads.push(resolve);
        }),
    });
    open();
    await untilSession("session-1");
    await vi.waitFor(() => {
      expect(loads).toHaveLength(1);
    });

    engine.close();
    loads[0]?.(player);

    await vi.waitFor(() => {
      expect(player.destroy).toHaveBeenCalledOnce();
    });
    expect(player.attachMedia).not.toHaveBeenCalled();
    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
  });

  it("ignores a session request that fails after close", async () => {
    let fail: () => void = () => undefined;
    const { engine, open, server, snapshot } = createEngineHarness({
      createSession: () =>
        new Promise<Response>((resolve) => {
          fail = () => {
            resolve(problemResponse(503, "session_limit_reached"));
          };
        }),
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.close();
    fail();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(snapshot()).toBe(initialPlaybackSnapshot);
  });

  it("ignores hls.js failing to load after close", async () => {
    const failures: ((reason: Error) => void)[] = [];
    const { engine, open, snapshot, untilSession } = createEngineHarness({
      delivery: "hls",
      createHls: () =>
        new Promise((_resolve, reject) => {
          failures.push(reject);
        }),
    });
    open();
    await untilSession("session-1");
    await vi.waitFor(() => {
      expect(failures).toHaveLength(1);
    });

    engine.close();
    failures[0]?.(new Error("chunk failed to load"));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(snapshot()).toBe(initialPlaybackSnapshot);
  });

  it("does nothing on close, retry, suspend or resume before open", () => {
    const { engine, server, snapshot } = createEngineHarness();

    engine.close();
    engine.retry();
    engine.suspend();
    engine.resume();

    expect(snapshot()).toBe(initialPlaybackSnapshot);
    expect(server.requests).toHaveLength(0);
  });

  it("closes the previous file when opened again", async () => {
    const { open, server, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");

    open(5);

    await untilSession("session-2");
    expect(server.stoppedSessions).toEqual(["session-1"]);
    expect(server.sessionRequests[1]?.replacesSessionId).toBeUndefined();
  });
});

describe("PlaybackEngine page lifecycle", () => {
  it("stops the session on suspend and restarts paused at the last position on resume", async () => {
    const { engine, open, media, server, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");
    media.emit("loadeddata");
    media.playTo(33);
    const playCalls = media.calls.filter((call) => call === "play").length;

    engine.suspend();
    engine.suspend();
    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(media.src).toBe("");

    engine.resume();
    engine.resume();
    await untilSession("session-2");
    expect(server.sessionRequests).toHaveLength(2);
    expect(server.sessionRequests[1]).toEqual(
      expect.objectContaining({ startPositionSeconds: 33 }),
    );
    expect(server.sessionRequests[1]?.replacesSessionId).toBeUndefined();
    expect(media.calls.filter((call) => call === "play")).toHaveLength(playCalls);
  });

  it("stops a session that arrives while the page is hidden", async () => {
    const answers = createHeldAnswers("file");
    const { engine, open, media, server, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.suspend();
    await answers.release(0);

    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(media.src).toBe("");
    engine.resume();
    await answers.release(1);
    await untilSession("session-2");
    expect(server.sessionRequests[1]?.replacesSessionId).toBeUndefined();
  });

  it("uses a session that arrives after the page was hidden and shown again", async () => {
    const answers = createHeldAnswers("file");
    const { engine, open, media, server, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open(30);
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.suspend();
    engine.resume();
    await answers.release(0);

    await untilSession("session-1");
    expect(media.src).toBe("/api/v1/playback/sessions/session-1/file");
    expect(server.sessionRequests).toHaveLength(1);
    expect(server.stoppedSessions).toEqual([]);
    // Shown again after a hide: the restart waits for the user's play.
    expect(media.calls).not.toContain("play");
  });

  it("uses an HLS session that arrives after hide and show at the same position", async () => {
    const answers = createHeldAnswers("hls");
    const { engine, open, server, untilSession } = createEngineHarness({
      createSession: answers.respond,
    });
    open(30);
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });

    engine.suspend();
    engine.resume();
    await answers.release(0);

    await untilSession("session-1");
    expect(server.sessionRequests).toHaveLength(1);
    expect(server.stoppedSessions).toEqual([]);
  });

  it("ignores seek and retry while the page is hidden", async () => {
    const { engine, open, media, server, untilSession } = createEngineHarness();
    open();
    await untilSession("session-1");
    media.emit("loadeddata");
    engine.suspend();

    engine.seek(50);
    engine.retry();

    await vi.waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(server.sessionRequests).toHaveLength(1);
  });
});
