import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient, jsonResponse } from "../../../test/fakeApi";
import { createPlaybackServer } from "../test/fakePlaybackServer";
import { fakeSession } from "../test/playerFixtures";
import { liveSessionIntervalMilliseconds, useLiveTranscodeSpeed } from "./useLiveTranscodeSpeed";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function setUp(speeds: readonly (number | undefined)[]) {
  let answered = 0;
  const server = createPlaybackServer({
    getSession: (sessionId) => {
      const transcodeSpeed = speeds[Math.min(answered, speeds.length - 1)];
      answered += 1;
      return jsonResponse({
        ...fakeSession({ id: sessionId }),
        ...(transcodeSpeed === undefined ? {} : { transcodeSpeed }),
      });
    },
  });
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={createFakeApiClient(server.fetch)}>
      <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  return { server, wrapper };
}

describe("useLiveTranscodeSpeed", () => {
  it("does not ask the server while the overlay is closed", async () => {
    const { server, wrapper } = setUp([2]);

    const { result } = renderHook(() => useLiveTranscodeSpeed("s1", false), { wrapper });
    await Promise.resolve();

    expect(result.current).toBeUndefined();
    expect(server.lookedUpSessions).toEqual([]);
  });

  it("reads the speed of the session once the overlay is open", async () => {
    const { server, wrapper } = setUp([2.4]);

    const { result } = renderHook(() => useLiveTranscodeSpeed("s1", true), { wrapper });

    await waitFor(() => {
      expect(result.current).toBe(2.4);
    });
    expect(server.lookedUpSessions).toEqual(["s1"]);
  });

  it("asks again every five seconds", async () => {
    expect(liveSessionIntervalMilliseconds).toBe(5000);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { wrapper } = setUp([2, 0.5]);

    const { result } = renderHook(() => useLiveTranscodeSpeed("s1", true), { wrapper });
    await waitFor(() => {
      expect(result.current).toBe(2);
    });
    await vi.advanceTimersByTimeAsync(liveSessionIntervalMilliseconds);

    await waitFor(() => {
      expect(result.current).toBe(0.5);
    });
  });

  it("stays unknown when the server cannot answer", async () => {
    const wrapperServer = createPlaybackServer();
    const wrapper = ({ children }: { readonly children: ReactNode }) => (
      <ApiClientProvider client={createFakeApiClient(wrapperServer.fetch)}>
        <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
      </ApiClientProvider>
    );

    const { result } = renderHook(() => useLiveTranscodeSpeed("gone", true), { wrapper });
    await waitFor(() => {
      expect(wrapperServer.lookedUpSessions).toEqual(["gone"]);
    });

    expect(result.current).toBeUndefined();
  });

  it("has no speed without a session", () => {
    const { server, wrapper } = setUp([2]);

    const { result } = renderHook(() => useLiveTranscodeSpeed(undefined, true), { wrapper });

    expect(result.current).toBeUndefined();
    expect(server.lookedUpSessions).toEqual([]);
  });
});
