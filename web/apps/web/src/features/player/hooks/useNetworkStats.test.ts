import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  networkStatsIntervalMilliseconds,
  resourceTimingBufferSize,
  useNetworkStats,
} from "./useNetworkStats";

const streamUrl = "/api/v1/playback/sessions/s1/hls/index.m3u8";

function fakeVideo(currentTime: number, bufferedEnd: number): HTMLVideoElement {
  const buffered = { length: 1, start: () => 0, end: () => bufferedEnd };
  return { currentTime, buffered } as unknown as HTMLVideoElement;
}

function fragment(name: string, encodedBodySize: number, duration: number) {
  return {
    entryType: "resource",
    name: new URL(name, window.location.href).href,
    encodedBodySize,
    duration,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("useNetworkStats", () => {
  it("is unknown while inactive and does not read the network", () => {
    const entries = vi.spyOn(performance, "getEntriesByType");

    const { result } = renderHook(() => useNetworkStats(fakeVideo(0, 10), streamUrl, false));

    expect(result.current).toEqual({
      bufferAheadSeconds: undefined,
      throughputBytesPerSecond: undefined,
    });
    expect(entries).not.toHaveBeenCalled();
  });

  it("reads buffer ahead and throughput when active and again every second", () => {
    const entries = vi
      .spyOn(performance, "getEntriesByType")
      .mockReturnValue([
        fragment("/api/v1/playback/sessions/s1/hls/seg-1.m4s", 2_000_000, 500),
      ] as unknown as PerformanceEntryList);
    const video = fakeVideo(4, 34);

    const { result } = renderHook(() => useNetworkStats(video, streamUrl, true));
    act(() => {
      vi.advanceTimersByTime(0);
    });

    expect(result.current).toEqual({
      bufferAheadSeconds: 30,
      throughputBytesPerSecond: 4_000_000,
    });
    const callsAfterFirstRead = entries.mock.calls.length;
    act(() => {
      vi.advanceTimersByTime(networkStatsIntervalMilliseconds);
    });
    expect(entries.mock.calls.length).toBe(callsAfterFirstRead + 1);
  });

  it("keeps the throughput unknown when no fragment has arrived", () => {
    vi.spyOn(performance, "getEntriesByType").mockReturnValue([]);

    const video = fakeVideo(0, 8);
    const { result } = renderHook(() => useNetworkStats(video, streamUrl, true));
    act(() => {
      vi.advanceTimersByTime(0);
    });

    expect(result.current.bufferAheadSeconds).toBe(8);
    expect(result.current.throughputBytesPerSecond).toBeUndefined();
  });

  it("forgets the numbers when the overlay closes", () => {
    vi.spyOn(performance, "getEntriesByType").mockReturnValue([]);
    const video = fakeVideo(0, 8);
    const { result, rerender } = renderHook(
      ({ active }) => useNetworkStats(video, streamUrl, active),
      {
        initialProps: { active: true },
      },
    );
    act(() => {
      vi.advanceTimersByTime(0);
    });
    expect(result.current.bufferAheadSeconds).toBe(8);

    rerender({ active: false });
    rerender({ active: true });

    expect(result.current.bufferAheadSeconds).toBeUndefined();
  });

  describe("Resource Timing buffer", () => {
    const setSize = vi.fn();
    // jsdom's performance is no event target; browsers' is.
    const events = new EventTarget();

    beforeEach(() => {
      setSize.mockClear();
      const define = (name: string, value: unknown) => {
        Object.defineProperty(performance, name, { value, configurable: true, writable: true });
      };
      define("setResourceTimingBufferSize", setSize);
      define("addEventListener", events.addEventListener.bind(events));
      define("removeEventListener", events.removeEventListener.bind(events));
      define("dispatchEvent", events.dispatchEvent.bind(events));
    });

    afterEach(() => {
      for (const name of [
        "setResourceTimingBufferSize",
        "addEventListener",
        "removeEventListener",
        "dispatchEvent",
      ]) {
        Reflect.deleteProperty(performance, name);
      }
    });

    it("is enlarged while sampling and doubled each time it reports full", () => {
      vi.spyOn(performance, "getEntriesByType").mockReturnValue([]);

      const { unmount } = renderHook(() => useNetworkStats(fakeVideo(0, 8), streamUrl, true));
      expect(setSize).toHaveBeenLastCalledWith(resourceTimingBufferSize);
      performance.dispatchEvent(new Event("resourcetimingbufferfull"));
      expect(setSize).toHaveBeenLastCalledWith(resourceTimingBufferSize * 2);

      unmount();
      performance.dispatchEvent(new Event("resourcetimingbufferfull"));
      expect(setSize).toHaveBeenCalledTimes(2);
    });

    it("is left alone while the overlay is closed", () => {
      renderHook(() => useNetworkStats(fakeVideo(0, 8), streamUrl, false));

      expect(setSize).not.toHaveBeenCalled();
    });
  });
});
