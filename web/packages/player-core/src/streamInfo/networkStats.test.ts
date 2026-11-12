import { describe, expect, it } from "vitest";

import type { TimeRangesLike } from "../timeline/timeRanges";
import {
  bufferAheadSeconds,
  throughputBytesPerSecond,
  throughputWindowSize,
  type TransferSample,
} from "./networkStats";

function ranges(list: readonly (readonly [number, number])[]): TimeRangesLike {
  return {
    length: list.length,
    start: (index) => list[index]?.[0] ?? Number.NaN,
    end: (index) => list[index]?.[1] ?? Number.NaN,
  };
}

describe("bufferAheadSeconds", () => {
  it.each([
    { name: "inside a range", list: [[0, 30]], time: 10, expected: 20 },
    {
      name: "picks the range holding the time",
      list: [
        [0, 5],
        [8, 40],
      ],
      time: 12,
      expected: 28,
    },
    { name: "time at the range end", list: [[0, 30]], time: 30, expected: 0 },
    {
      name: "time in a gap",
      list: [
        [0, 5],
        [8, 40],
      ],
      time: 6,
      expected: 0,
    },
    { name: "nothing buffered", list: [], time: 3, expected: 0 },
  ] as const)("$name", ({ list, time, expected }) => {
    expect(bufferAheadSeconds(ranges(list), time)).toBe(expected);
  });

  it("is unknown for a missing element or an unusable time", () => {
    expect(bufferAheadSeconds(undefined, 1)).toBeUndefined();
    expect(bufferAheadSeconds(ranges([[0, 5]]), Number.NaN)).toBeUndefined();
  });
});

function sample(bytes: number, seconds: number): TransferSample {
  return { bytes, durationSeconds: seconds };
}

describe("throughputBytesPerSecond", () => {
  it.each([
    { name: "one transfer", samples: [sample(1_000_000, 0.5)], expected: 2_000_000 },
    {
      name: "weights by time, not by count",
      samples: [sample(1_000_000, 1), sample(3_000_000, 1)],
      expected: 2_000_000,
    },
    {
      name: "skips cached or empty transfers",
      samples: [sample(0, 0.2), sample(500_000, 0.25), sample(100, 0)],
      expected: 2_000_000,
    },
    {
      name: "skips unusable numbers",
      samples: [sample(Number.NaN, 1), sample(10, -1)],
      expected: undefined,
    },
    { name: "no transfers", samples: [], expected: undefined },
  ])("$name", ({ samples, expected }) => {
    expect(throughputBytesPerSecond(samples)).toBe(expected);
  });

  it("uses only the most recent transfers", () => {
    const old = Array.from({ length: 10 }, () => sample(1000, 1));
    const recent = Array.from({ length: throughputWindowSize }, () => sample(9000, 1));

    expect(throughputBytesPerSecond([...old, ...recent])).toBe(9000);
  });
});
