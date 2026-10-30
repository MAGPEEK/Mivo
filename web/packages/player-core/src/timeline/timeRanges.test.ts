import { describe, expect, it } from "vitest";

import { rangeContains, readTimeRanges, type TimeRangesLike } from "./timeRanges";

function fakeTimeRanges(ranges: readonly (readonly [number, number])[]): TimeRangesLike {
  return {
    length: ranges.length,
    start: (index) => ranges[index]?.[0] ?? Number.NaN,
    end: (index) => ranges[index]?.[1] ?? Number.NaN,
  };
}

describe("readTimeRanges", () => {
  it("copies every range in order", () => {
    expect(
      readTimeRanges(
        fakeTimeRanges([
          [0, 12],
          [30, 48.5],
        ]),
      ),
    ).toEqual([
      { start: 0, end: 12 },
      { start: 30, end: 48.5 },
    ]);
  });

  it("returns no ranges for an empty object", () => {
    expect(readTimeRanges(fakeTimeRanges([]))).toEqual([]);
  });
});

describe("rangeContains", () => {
  it.each([
    { time: 10, expected: true },
    { time: 15, expected: true },
    { time: 20, expected: true },
    { time: 9.99, expected: false },
    { time: 20.01, expected: false },
    { time: Number.NaN, expected: false },
  ])("is $expected for $time in [10, 20]", ({ time, expected }) => {
    expect(rangeContains({ start: 10, end: 20 }, time)).toBe(expected);
  });
});
