import type { ClientProfile } from "@mivo/player-core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient, problemResponse } from "../../../test/fakeApi";
import type { HlsFactory } from "../lib/hlsPlayer";
import { createPlaybackServer, type PlaybackServer } from "../test/fakePlaybackServer";
import { createManualTimers } from "../test/fakeMedia";
import { mseProfile, playerFileId, playerItem, playerItemId } from "../test/playerFixtures";
import {
  usePlaybackSession,
  type PlaybackServices,
  type PlaybackTarget,
} from "./usePlaybackSession";

const target: PlaybackTarget = { itemId: playerItemId, fileId: undefined, startAt: undefined };

interface RenderOptions {
  readonly server?: PlaybackServer;
  readonly services?: PlaybackServices;
  readonly queryClient?: QueryClient;
}

function renderPlayback(initialTarget: PlaybackTarget = target, options: RenderOptions = {}) {
  const {
    server = createPlaybackServer(),
    queryClient = new QueryClient(),
    services = {
      detectProfile: () => Promise.resolve(mseProfile),
      startTimer: createManualTimers().startTimer,
    },
  } = options;
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={createFakeApiClient(server.fetch)}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  const rendered = renderHook(
    ({ playbackTarget }: { playbackTarget: PlaybackTarget }) =>
      usePlaybackSession(playbackTarget, services),
    { wrapper, initialProps: { playbackTarget: initialTarget } },
  );
  const video = document.createElement("video");
  act(() => {
    rendered.result.current.attachVideo(video);
  });
  return { ...rendered, server, video };
}

beforeEach(() => {
  // jsdom implements neither; the engine calls both.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  // Unmount first: closing the engine empties the element, which calls load().
  cleanup();
  vi.restoreAllMocks();
});

describe("usePlaybackSession", () => {
  it("plays the item's primary file in the attached video element", async () => {
    const { result, server, video } = renderPlayback();

    await waitFor(() => {
      expect(result.current.session?.id).toBe("session-1");
    });

    expect(server.sessionRequests).toEqual([{ fileId: playerFileId, profile: mseProfile }]);
    expect(video.getAttribute("src")).toBe(result.current.session?.streamUrl);
    act(() => {
      video.dispatchEvent(new Event("loadeddata"));
    });
    expect(result.current.status).toBe("playing");
  });

  it("plays the requested file from the clamped start position", async () => {
    const { result, server } = renderPlayback({ ...target, fileId: playerFileId, startAt: 1e21 });

    await waitFor(() => {
      expect(result.current.session).toBeDefined();
    });

    expect(server.sessionRequests[0]?.startPositionSeconds).toBe(119);
  });

  it("seeks and retries through the engine", async () => {
    const { result, server, video } = renderPlayback();
    await waitFor(() => {
      expect(result.current.session).toBeDefined();
    });
    act(() => {
      video.dispatchEvent(new Event("loadeddata"));
    });

    act(() => {
      result.current.seek(40);
    });
    expect(video.currentTime).toBe(40);
    act(() => {
      result.current.retry();
    });

    await waitFor(() => {
      expect(result.current.session?.id).toBe("session-2");
    });
    expect(server.sessionRequests[1]).toMatchObject({ replacesSessionId: "session-1" });
  });

  it("stops the session with a keepalive request on unmount", async () => {
    const { result, server, unmount } = renderPlayback();
    await waitFor(() => {
      expect(result.current.session).toBeDefined();
    });

    unmount();

    await waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    expect(server.stopRequests[0]?.keepalive).toBe(true);
  });

  it("stops the session on pagehide and restarts it when the cached page returns", async () => {
    const { result, server } = renderPlayback();
    await waitFor(() => {
      expect(result.current.session).toBeDefined();
    });

    act(() => {
      window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: true }));
    });
    await waitFor(() => {
      expect(server.stoppedSessions).toEqual(["session-1"]);
    });
    act(() => {
      window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: false }));
    });
    expect(server.sessionRequests).toHaveLength(1);
    act(() => {
      window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
    });

    await waitFor(() => {
      expect(result.current.session?.id).toBe("session-2");
    });
  });

  it("detects the client profile once per page load", async () => {
    const detectProfile = vi.fn(() => Promise.resolve(mseProfile));
    const queryClient = new QueryClient();
    const services = { detectProfile, startTimer: createManualTimers().startTimer };

    const first = renderPlayback(target, { queryClient, services });
    await waitFor(() => {
      expect(first.result.current.session).toBeDefined();
    });
    first.unmount();
    const second = renderPlayback(target, { queryClient, services });
    await waitFor(() => {
      expect(second.result.current.session).toBeDefined();
    });

    expect(detectProfile).toHaveBeenCalledOnce();
  });

  it("shows a generic error when the item cannot be loaded and loads it again on retry", async () => {
    let itemAnswers = 0;
    const server = createPlaybackServer();
    const failingFirst = createPlaybackServer();
    const fetch = (request: Request) => {
      if (new URL(request.url).pathname === `/api/v1/items/${playerItemId}`) {
        itemAnswers += 1;
        if (itemAnswers === 1) {
          return Promise.resolve(problemResponse(503, "unavailable"));
        }
      }
      return server.fetch(request);
    };
    const { result } = renderPlayback(target, { server: { ...failingFirst, fetch } });

    await waitFor(() => {
      expect(result.current).toMatchObject({ status: "error", error: { code: "generic" } });
    });
    act(() => {
      result.current.retry();
    });

    await waitFor(() => {
      expect(result.current.session?.id).toBe("session-1");
    });
    expect(itemAnswers).toBe(2);
  });

  it("shows a generic error when profile detection fails and detects again on retry", async () => {
    const detectProfile = vi
      .fn<() => Promise<ClientProfile>>()
      .mockRejectedValueOnce(new Error("probe failed"))
      .mockResolvedValue(mseProfile);
    const { result } = renderPlayback(target, { services: { detectProfile } });

    await waitFor(() => {
      expect(result.current.status).toBe("error");
    });
    act(() => {
      result.current.retry();
    });

    await waitFor(() => {
      expect(result.current.session?.id).toBe("session-1");
    });
    expect(detectProfile).toHaveBeenCalledTimes(2);
  });

  it("shows a generic error for an item without files", async () => {
    const server = createPlaybackServer({ item: playerItem({ files: [] }) });

    const { result } = renderPlayback(target, { server });

    await waitFor(() => {
      expect(result.current.status).toBe("error");
    });
    expect(server.sessionRequests).toEqual([]);
  });

  it("uses the browser's profile and hls.js by default", async () => {
    const { result, server } = renderPlayback(target, { services: {} });

    await waitFor(() => {
      expect(result.current.session).toBeDefined();
    });

    // jsdom plays nothing: the detected profile has no direct play and no HLS.
    const profile: ClientProfile | undefined = server.sessionRequests[0]?.profile;
    expect(profile).toMatchObject({ directPlay: [], hls: { delivery: "none" } });
  });

  it("keeps a custom hls.js factory", async () => {
    const createHls = vi.fn<HlsFactory<HTMLVideoElement>>(() =>
      Promise.resolve({
        attachMedia: vi.fn(),
        loadSource: vi.fn(),
        recoverMediaError: vi.fn(),
        destroy: vi.fn(),
      }),
    );
    const server = createPlaybackServer({ delivery: "hls" });

    renderPlayback(target, {
      server,
      services: { createHls, detectProfile: () => Promise.resolve(mseProfile) },
    });

    await waitFor(() => {
      expect(createHls).toHaveBeenCalledOnce();
    });
  });
});
