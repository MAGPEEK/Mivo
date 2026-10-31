import { describe, expect, it } from "vitest";

import { clockTime, displayedClock, needsHours, spokenClock } from "./playbackTime";

describe("clockTime", () => {
  it.each([
    { seconds: 0, withHours: false, expected: "0:00" },
    { seconds: 5.9, withHours: false, expected: "0:05" },
    { seconds: 65, withHours: false, expected: "1:05" },
    { seconds: 3599, withHours: false, expected: "59:59" },
    // A position past an hour never hides its hours, even in the short layout.
    { seconds: 3725, withHours: false, expected: "1:02:05" },
    { seconds: 65, withHours: true, expected: "0:01:05" },
    { seconds: 36_000, withHours: true, expected: "10:00:00" },
  ])("formats $seconds s as $expected (hours: $withHours)", ({ seconds, withHours, expected }) => {
    expect(clockTime(seconds, withHours)).toBe(expected);
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, -3])("reads %s as zero", (seconds) => {
    expect(clockTime(seconds, true)).toBe("0:00:00");
  });
});

describe("needsHours", () => {
  it.each([
    { duration: undefined, expected: false },
    { duration: 3599.9, expected: false },
    { duration: 3600, expected: true },
  ])("is $expected for $duration s", ({ duration, expected }) => {
    expect(needsHours(duration)).toBe(expected);
  });
});

describe("displayedClock", () => {
  it("uses one layout for position and duration", () => {
    expect(displayedClock(65, 120)).toEqual({ position: "1:05", duration: "2:00" });
    expect(displayedClock(65, 5400)).toEqual({ position: "0:01:05", duration: "1:30:00" });
  });

  it("leaves out an unknown duration", () => {
    expect(displayedClock(65, undefined)).toEqual({ position: "1:05", duration: undefined });
  });
});

describe("spokenClock", () => {
  it("always reads hours, minutes and seconds", () => {
    expect(spokenClock(65, 120)).toEqual({ position: "0:01:05", duration: "0:02:00" });
    expect(spokenClock(65, undefined)).toEqual({ position: "0:01:05", duration: undefined });
  });
});
