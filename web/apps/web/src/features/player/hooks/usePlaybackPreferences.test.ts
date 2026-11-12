import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { autoPlayStorageKey } from "../lib/autoPlayPreference";
import { speedStorageKey } from "../lib/playbackSpeed";
import { usePlaybackPreferences } from "./usePlaybackPreferences";

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  window.localStorage.clear();
});

describe("usePlaybackPreferences", () => {
  it("starts with auto play on and normal speed", () => {
    const { result } = renderHook(() => usePlaybackPreferences(null));
    expect(result.current.autoPlay).toBe(true);
    expect(result.current.speed).toBe(1);
  });

  it("reads saved values and ignores invalid ones", () => {
    window.localStorage.setItem(autoPlayStorageKey, "false");
    window.localStorage.setItem(speedStorageKey, "1.75");
    expect(renderHook(() => usePlaybackPreferences(null)).result.current).toMatchObject({
      autoPlay: false,
      speed: 1.75,
    });

    window.localStorage.setItem(autoPlayStorageKey, "maybe");
    window.localStorage.setItem(speedStorageKey, "7");
    expect(renderHook(() => usePlaybackPreferences(null)).result.current).toMatchObject({
      autoPlay: true,
      speed: 1,
    });
  });

  it("saves the choices", () => {
    const { result } = renderHook(() => usePlaybackPreferences(null));
    act(() => {
      result.current.setAutoPlay(false);
      result.current.setSpeed(0.75);
    });
    expect(result.current).toMatchObject({ autoPlay: false, speed: 0.75 });
    expect(window.localStorage.getItem(autoPlayStorageKey)).toBe("false");
    expect(window.localStorage.getItem(speedStorageKey)).toBe("0.75");
  });

  it("applies the speed to the element, now and after a change", () => {
    const video = document.createElement("video");
    window.localStorage.setItem(speedStorageKey, "1.5");
    const { result } = renderHook(() => usePlaybackPreferences(video));
    expect(video.playbackRate).toBe(1.5);
    expect(video.defaultPlaybackRate).toBe(1.5);
    expect(video.preservesPitch).toBe(true);

    act(() => {
      result.current.setSpeed(2);
    });
    expect(video.playbackRate).toBe(2);
  });
});
