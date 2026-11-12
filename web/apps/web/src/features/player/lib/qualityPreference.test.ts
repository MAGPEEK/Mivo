import { qualityOptions } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import {
  availableQualityChoice,
  defaultQualityChoice,
  isChosen,
  qualityStorageKey,
  readQualityChoice,
  startingMaxBitRate,
  writeQualityChoice,
  type QualityChoice,
} from "./qualityPreference";

function memoryStorage(initial?: string): Storage {
  const values = new Map<string, string>(
    initial === undefined ? [] : [[qualityStorageKey, initial]],
  );
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      values.set(key, value);
    },
  } as Storage;
}

const options = qualityOptions(14_200_000, 1080);
const eightMegabits = 8_000_000;

describe("quality preference storage", () => {
  it.each<[string, QualityChoice]>([
    ["original", { kind: "original" }],
    ["automatic", { kind: "automatic" }],
    [String(eightMegabits), { kind: "limited", maxBitRate: eightMegabits }],
  ])("round-trips %s", (stored, choice) => {
    const storage = memoryStorage();

    writeQualityChoice(storage, choice);

    expect(storage.getItem(qualityStorageKey)).toBe(stored);
    expect(readQualityChoice(storage)).toEqual(choice);
  });

  it.each(["", "0", "-4", "1.5", "fast", "NaN"])("reads %j as the original", (stored) => {
    expect(readQualityChoice(memoryStorage(stored))).toEqual(defaultQualityChoice);
  });

  it("reads the original without storage or when reading throws", () => {
    const failing = {
      getItem: () => {
        throw new Error("blocked");
      },
    } as unknown as Storage;

    expect(readQualityChoice(undefined)).toEqual(defaultQualityChoice);
    expect(readQualityChoice(failing)).toEqual(defaultQualityChoice);
  });

  it("survives a failing write", () => {
    const failing = {
      setItem: () => {
        throw new Error("quota");
      },
    } as unknown as Storage;

    expect(() => {
      writeQualityChoice(failing, { kind: "automatic" });
    }).not.toThrow();
  });
});

describe("quality choices", () => {
  it("drops a step the file does not offer", () => {
    const aboveSource = { kind: "limited", maxBitRate: 20_000_000 } as const;
    const offered = { kind: "limited", maxBitRate: eightMegabits } as const;

    expect(availableQualityChoice(aboveSource, options)).toEqual(defaultQualityChoice);
    expect(availableQualityChoice(offered, options)).toEqual(offered);
    expect(availableQualityChoice({ kind: "automatic" }, options)).toEqual({ kind: "automatic" });
  });

  it("starts Automatic with the original and a step with its limit", () => {
    expect(startingMaxBitRate({ kind: "automatic" })).toBe(0);
    expect(startingMaxBitRate({ kind: "original" })).toBe(0);
    expect(startingMaxBitRate({ kind: "limited", maxBitRate: eightMegabits })).toBe(eightMegabits);
  });

  it("marks the option a choice asks for, none for Automatic", () => {
    const [original, ...steps] = options;
    const eight = steps.find((step) => step.maxBitRate === eightMegabits);

    expect(original && isChosen({ kind: "original" }, original)).toBe(true);
    expect(eight && isChosen({ kind: "limited", maxBitRate: eightMegabits }, eight)).toBe(true);
    expect(original && isChosen({ kind: "automatic" }, original)).toBe(false);
  });
});
