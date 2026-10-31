import type { TimeRange, TimeRangesLike } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import { fakeSession } from "../test/playerFixtures";
import { producedRanges, sameRanges, trackPercent, trackSpan } from "./scrubberTrack";

function timeRanges(ranges: readonly TimeRange[]): TimeRangesLike {
  return {
    length: ranges.length,
    start: (index) => ranges[index]?.start ?? Number.NaN,
    end: (index) => ranges[index]?.end ?? Number.NaN,
  };
}

describe("producedRanges", () => {
  it("maps the seekable ranges of an HLS stream onto the file timeline", () => {
    const session = fakeSession({ id: "s", delivery: "hls", startPositionSeconds: 60 });

    expect(producedRanges(session, timeRanges([{ start: 0, end: 18 }]))).toEqual([
      { start: 60, end: 78 },
    ]);
  });

  it("shades nothing for a direct-play file or before the first session", () => {
    const seekable = timeRanges([{ start: 0, end: 120 }]);

    expect(producedRanges(fakeSession({ id: "s" }), seekable)).toEqual([]);
    expect(producedRanges(undefined, seekable)).toEqual([]);
  });
});

describe("sameRanges", () => {
  it.each([
    { first: [], second: [], expected: true },
    { first: [{ start: 0, end: 6 }], second: [{ start: 0, end: 6 }], expected: true },
    { first: [{ start: 0, end: 6 }], second: [{ start: 0, end: 12 }], expected: false },
    { first: [{ start: 0, end: 6 }], second: [{ start: 1, end: 6 }], expected: false },
    { first: [{ start: 0, end: 6 }], second: [], expected: false },
  ])("is $expected for $first and $second", ({ first, second, expected }) => {
    expect(sameRanges(first, second)).toBe(expected);
  });
});

describe("trackPercent", () => {
  it.each([
    { seconds: 30, duration: 120, expected: 25 },
    { seconds: 240, duration: 120, expected: 100 },
    { seconds: -5, duration: 120, expected: 0 },
    { seconds: Number.NaN, duration: 120, expected: 0 },
    { seconds: 30, duration: undefined, expected: 0 },
    { seconds: 30, duration: 0, expected: 0 },
  ])("places $seconds s of $duration s at $expected %", ({ seconds, duration, expected }) => {
    expect(trackPercent(seconds, duration)).toBe(expected);
  });
});

describe("trackSpan", () => {
  it("returns start and width in percent", () => {
    expect(trackSpan({ start: 30, end: 60 }, 120)).toEqual({ startPercent: 25, widthPercent: 25 });
  });
});
