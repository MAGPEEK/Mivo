import { describe, expect, it, vi } from "vitest";

import {
  autoPlayStorageKey,
  defaultAutoPlay,
  readAutoPlay,
  writeAutoPlay,
} from "./autoPlayPreference";

function storageWith(value: string | null): Storage {
  const data = new Map<string, string>(value === null ? [] : [[autoPlayStorageKey, value]]);
  return {
    get length() {
      return data.size;
    },
    clear: () => {
      data.clear();
    },
    getItem: (key) => data.get(key) ?? null,
    key: () => null,
    removeItem: (key) => {
      data.delete(key);
    },
    setItem: (key, stored) => {
      data.set(key, stored);
    },
  };
}

describe("auto play preference", () => {
  it("is on by default", () => {
    expect(defaultAutoPlay).toBe(true);
    expect(readAutoPlay(storageWith(null))).toBe(true);
  });

  it("round-trips both values", () => {
    const storage = storageWith(null);
    writeAutoPlay(storage, false);
    expect(readAutoPlay(storage)).toBe(false);
    writeAutoPlay(storage, true);
    expect(readAutoPlay(storage)).toBe(true);
  });

  it.each(["", "yes", "1", "null"])("falls back to the default for %j", (stored) => {
    expect(readAutoPlay(storageWith(stored))).toBe(defaultAutoPlay);
  });

  it("survives a missing or throwing storage", () => {
    const broken = storageWith(null);
    vi.spyOn(broken, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(broken, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    expect(readAutoPlay(undefined)).toBe(defaultAutoPlay);
    expect(readAutoPlay(broken)).toBe(defaultAutoPlay);
    expect(() => {
      writeAutoPlay(broken, false);
      writeAutoPlay(undefined, false);
    }).not.toThrow();
  });
});
