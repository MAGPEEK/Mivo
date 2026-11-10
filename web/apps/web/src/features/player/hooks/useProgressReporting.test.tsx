// The progress reports of the player hook, through the real engine, reporter and API client.

import { progressIntervalMilliseconds } from "@mivo/player-core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient, problemResponse } from "../../../test/fakeApi";
import { createPlaybackServer, type PlaybackServerOptions } from "../test/fakePlaybackServer";
import { createManualTimers } from "../test/fakeMedia";
import { mseProfile, playerItem, playerItemId } from "../test/playerFixtures";
import { usePlaybackSession, type PlaybackTarget } from "./usePlaybackSession";

const target: PlaybackTarget = { itemId: playerItemId, fileId: undefined, startAt: undefined };

/** Renders the hook, attaches a video element and waits until the stream plays. */
async function renderPlaying(
  serverOptions: PlaybackServerOptions = {},
  playbackTarget: PlaybackTarget = target,
) {
  const server = createPlaybackServer(serverOptions);
  const timers = createManualTimers();
  const services = {
    detectProfile: () => Promise.resolve(mseProfile),
    startTimer: timers.startTimer,
    // hls.js is not needed: the stream is "attached" once the factory answered.
    createHls: () =>
      Promise.resolve({
        attachMedia: () => undefined,
        loadSource: () => undefined,
        recoverMediaError: () => undefined,
        destroy: () => undefined,
      }),
  };
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={createFakeApiClient(server.fetch)}>
      <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  const rendered = renderHook(() => usePlaybackSession(playbackTarget, services), { wrapper });
  const video = document.createElement("video");
  act(() => {
    rendered.result.current.attachVideo(video);
  });
  // The stream is attached some time after the session arrived (hls.js loads lazily).
  await waitFor(() => {
    act(() => {
      video.dispatchEvent(new Event("loadedmetadata"));
      video.dispatchEvent(new Event("loadeddata"));
    });
    expect(rendered.result.current.status).toBe("playing");
  });
  const emit = (type: string, currentTime?: number) => {
    act(() => {
      if (currentTime !== undefined) {
        video.currentTime = currentTime;
      }
      video.dispatchEvent(new Event(type));
    });
  };
  return { ...rendered, server, timers, emit };
}

beforeEach(() => {
  // jsdom implements none of them; the engine calls them.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  // Unmount first: closing the engine empties the element, which calls load().
  cleanup();
  vi.restoreAllMocks();
});

describe("usePlaybackSession progress reports", () => {
  it("reports the position with the session on pause, with keepalive", async () => {
    const { server, emit } = await renderPlaying();

    emit("timeupdate", 12);
    emit("pause");

    await waitFor(() => {
      expect(server.progressReports).toEqual([
        { positionSeconds: 12, playbackSessionId: "session-1" },
      ]);
    });
    expect(server.progressRequests[0]?.keepalive).toBe(true);
  });

  it("reports every 10 seconds while playing", async () => {
    const { server, timers, emit } = await renderPlaying();

    emit("play");
    emit("timeupdate", 10);
    const [interval] = timers.pending();
    expect(interval?.milliseconds).toBe(progressIntervalMilliseconds);
    act(() => {
      interval?.fire();
    });

    await waitFor(() => {
      expect(server.progressReports).toEqual([
        { positionSeconds: 10, playbackSessionId: "session-1" },
      ]);
    });
    expect(timers.pending().map((timer) => timer.milliseconds)).toEqual([
      progressIntervalMilliseconds,
    ]);
  });

  it("adds the start position of an HLS session that began at ?t=", async () => {
    const { server, emit } = await renderPlaying({ delivery: "hls" }, { ...target, startAt: 60 });
    expect(server.sessionRequests[0]?.startPositionSeconds).toBe(60);

    emit("timeupdate", 5);
    emit("pause");

    await waitFor(() => {
      expect(server.progressReports.map((report) => report.positionSeconds)).toEqual([65]);
    });
  });

  it("reports the last position when the player is left, before the session stops", async () => {
    const { server, emit, unmount } = await renderPlaying();
    emit("timeupdate", 33);

    unmount();

    await waitFor(() => {
      expect(server.progressReports).toEqual([
        { positionSeconds: 33, playbackSessionId: "session-1" },
      ]);
    });
    expect(server.progressRequests[0]?.keepalive).toBe(true);
  });

  it("reports when the page is hidden", async () => {
    const { server, emit } = await renderPlaying();
    emit("timeupdate", 21);

    act(() => {
      window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: false }));
    });

    await waitFor(() => {
      expect(server.progressReports.map((report) => report.positionSeconds)).toEqual([21]);
    });
  });

  it("does not report music", async () => {
    const { server, emit, unmount } = await renderPlaying({ item: playerItem({ kind: "track" }) });

    emit("play");
    emit("timeupdate", 12);
    emit("pause");
    unmount();
    await waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });

    expect(server.progressReports).toEqual([]);
  });

  it.each([501, 401, 403, 500])("keeps playing when the server answers %i", async (status) => {
    const { server, result, emit } = await renderPlaying({
      reportProgress: () => problemResponse(status, "problem"),
    });

    emit("timeupdate", 12);
    emit("pause");
    await waitFor(() => {
      expect(server.progressReports).toHaveLength(1);
    });
    emit("timeupdate", 13);

    expect(result.current).toMatchObject({ status: "playing", error: undefined, position: 13 });
  });
});
