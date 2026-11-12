import { describe, expect, it, vi } from "vitest";

import {
  applyPlaybackSpeed,
  defaultPlaybackSpeed,
  isPlaybackSpeed,
  playbackSpeeds,
  readPlaybackSpeed,
  speedStorageKey,
  writePlaybackSpeed,
} from "./playbackSpeed";

function memoryStorage(initial: Record<string, string> = {}): Storage {
  const data = new Map(Object.entries(initial));
  return {
    get length() {
      return data.size;
    },
    clear: () => {
      data.clear();
    },
    getItem: (key) => data.get(key) ?? null,
    key: (index) => [...data.keys()][index] ?? null,
    removeItem: (key) => {
      data.delete(key);
    },
    setItem: (key, value) => {
      data.set(key, value);
    },
  };
}

describe("playback speed", () => {
  it("offers 0.5 to 2 in the agreed steps, with 1 as default", () => {
    expect(playbackSpeeds).toEqual([0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]);
    expect(defaultPlaybackSpeed).toBe(1);
  });

  it.each([
    [0.5, true],
    [1.75, true],
    [2, true],
    [1.1, false],
    [3, false],
    ["1", false],
    [Number.NaN, false],
    [undefined, false],
  ])("recognises %s as a speed: %s", (value, expected) => {
    expect(isPlaybackSpeed(value)).toBe(expected);
  });

  it("reads a saved speed back", () => {
    const storage = memoryStorage();
    writePlaybackSpeed(storage, 1.5);
    expect(storage.getItem(speedStorageKey)).toBe("1.5");
    expect(readPlaybackSpeed(storage)).toBe(1.5);
  });

  it.each(["", "fast", "1.1", "0", "NaN", "9"])("falls back to 1 for the stored %j", (stored) => {
    expect(readPlaybackSpeed(memoryStorage({ [speedStorageKey]: stored }))).toBe(1);
  });

  it("falls back to 1 without storage or when storage throws", () => {
    const broken = memoryStorage();
    vi.spyOn(broken, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(broken, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    expect(readPlaybackSpeed(undefined)).toBe(1);
    expect(readPlaybackSpeed(broken)).toBe(1);
    expect(() => {
      writePlaybackSpeed(broken, 2);
      writePlaybackSpeed(undefined, 2);
    }).not.toThrow();
  });

  it("sets the rate for the running and for the next source and keeps the pitch", () => {
    const media = { playbackRate: 1, defaultPlaybackRate: 1, preservesPitch: false };
    applyPlaybackSpeed(media, 1.25);
    expect(media).toEqual({ playbackRate: 1.25, defaultPlaybackRate: 1.25, preservesPitch: true });
  });
});
