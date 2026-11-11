import { describe, expect, it } from "vitest";

import { qualityLadder, qualityOptions } from "./qualityOptions";

/** `[maxBitRate in Mbit/s, height]` of every option; the original has bit rate 0. */
function summary(sourceBitRate: number | undefined, height: number | undefined) {
  return qualityOptions(sourceBitRate, height).map((option) => [
    option.maxBitRate / 1_000_000,
    option.height,
  ]);
}

describe("qualityLadder", () => {
  it("is the ladder of ADR 0030, best first", () => {
    expect(qualityLadder.map((step) => step.maxBitRate / 1_000_000)).toEqual([20, 12, 8, 4, 2, 1]);
    expect(new Set(qualityLadder.map((step) => step.height))).toEqual(
      new Set([2160, 1080, 720, 480]),
    );
  });
});

describe("qualityOptions", () => {
  it.each<{
    name: string;
    sourceBitRate: number | undefined;
    height: number | undefined;
    expected: [number, number | undefined][];
  }>([
    {
      name: "a 4K source above the ladder gets every step",
      sourceBitRate: 45_000_000,
      height: 2160,
      expected: [
        [0, 2160],
        [20, 2160],
        [12, 1080],
        [8, 1080],
        [4, 720],
        [2, 480],
        [1, 480],
      ],
    },
    {
      name: "only steps below the source",
      sourceBitRate: 8_000_000,
      height: 1080,
      expected: [
        [0, 1080],
        [4, 720],
        [2, 480],
        [1, 480],
      ],
    },
    {
      name: "a step never claims more height than the source",
      sourceBitRate: 9_000_000,
      height: 576,
      expected: [
        [0, 576],
        [8, 576],
        [4, 576],
        [2, 480],
        [1, 480],
      ],
    },
    {
      name: "a source below the ladder has the original only",
      sourceBitRate: 800_000,
      height: 360,
      expected: [[0, 360]],
    },
    {
      name: "an unknown bit rate rules out no step",
      sourceBitRate: undefined,
      height: 720,
      expected: [
        [0, 720],
        [20, 720],
        [12, 720],
        [8, 720],
        [4, 720],
        [2, 480],
        [1, 480],
      ],
    },
    {
      name: "an unknown height keeps the ladder's heights",
      sourceBitRate: 5_000_000,
      height: undefined,
      expected: [
        [0, undefined],
        [4, 720],
        [2, 480],
        [1, 480],
      ],
    },
    {
      name: "unusable numbers count as unknown",
      sourceBitRate: Number.NaN,
      height: 0,
      expected: [
        [0, undefined],
        [20, 2160],
        [12, 1080],
        [8, 1080],
        [4, 720],
        [2, 480],
        [1, 480],
      ],
    },
  ])("$name", ({ sourceBitRate, height, expected }) => {
    expect(summary(sourceBitRate, height)).toEqual(expected);
  });

  it("marks the original and carries the source bit rate", () => {
    const [original, step] = qualityOptions(6_000_000, 1080);
    expect(original).toEqual({
      kind: "original",
      maxBitRate: 0,
      sourceBitRate: 6_000_000,
      height: 1080,
    });
    expect(step).toEqual({ kind: "limited", maxBitRate: 4_000_000, height: 720 });
  });
});
