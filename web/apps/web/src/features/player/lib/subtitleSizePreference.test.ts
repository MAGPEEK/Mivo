import { describe, expect, it } from "vitest";

import {
  defaultSubtitleSize,
  readSubtitleSize,
  subtitleSizeClass,
  subtitleSizes,
  subtitleSizeStorageKey,
  writeSubtitleSize,
} from "./subtitleSizePreference";

function memoryStorage(initial?: string): Storage {
  const values = new Map<string, string>(
    initial === undefined ? [] : [[subtitleSizeStorageKey, initial]],
  );
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      values.set(key, value);
    },
  } as Storage;
}

describe("subtitle size preference", () => {
  it.each(subtitleSizes)("round-trips %s", (size) => {
    const storage = memoryStorage();

    writeSubtitleSize(storage, size);

    expect(readSubtitleSize(storage)).toBe(size);
  });

  it.each([undefined, "", "enormous", "Normal"])("reads %j as the default", (stored) => {
    expect(readSubtitleSize(memoryStorage(stored))).toBe(defaultSubtitleSize);
    expect(readSubtitleSize(undefined)).toBe(defaultSubtitleSize);
  });

  it("gives every size its own text classes", () => {
    const classes = subtitleSizes.map(subtitleSizeClass);

    expect(new Set(classes).size).toBe(subtitleSizes.length);
  });

  it("survives a storage that throws", () => {
    const failing = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("quota");
      },
    } as unknown as Storage;

    expect(readSubtitleSize(failing)).toBe(defaultSubtitleSize);
    expect(() => {
      writeSubtitleSize(failing, "huge");
    }).not.toThrow();
  });
});
