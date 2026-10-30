import { describe, expect, it } from "vitest";

import {
  clampPosition,
  displayedPosition,
  displayedRanges,
  knownDuration,
  mediaClockOffset,
  mediaTimeForPosition,
  type TimelineSession,
} from "./timeline";

const duration = 100;
const hls: TimelineSession = {
  delivery: "hls",
  startPositionSeconds: 30,
  durationSeconds: duration,
};
const file: TimelineSession = {
  delivery: "file",
  startPositionSeconds: 30,
  durationSeconds: duration,
};
const hlsWithoutDuration: TimelineSession = { delivery: "hls", startPositionSeconds: 30 };

describe("knownDuration", () => {
  it.each([
    { durationSeconds: 100, expected: 100 },
    { durationSeconds: 0, expected: 0 },
    { durationSeconds: undefined, expected: undefined },
    { durationSeconds: -1, expected: undefined },
    { durationSeconds: Number.NaN, expected: undefined },
    { durationSeconds: Number.POSITIVE_INFINITY, expected: undefined },
  ])("is $expected for a duration of $durationSeconds", ({ durationSeconds, expected }) => {
    const session: TimelineSession =
      durationSeconds === undefined
        ? { delivery: "hls", startPositionSeconds: 0 }
        : { delivery: "hls", startPositionSeconds: 0, durationSeconds };

    expect(knownDuration(session)).toBe(expected);
  });
});

describe("mediaClockOffset", () => {
  it.each([
    { name: "a progressive file", session: file, expected: 0 },
    { name: "an HLS stream", session: hls, expected: 30 },
    { name: "a delivery added later", session: { ...hls, delivery: "dash" }, expected: 30 },
    {
      name: "an HLS stream with a negative start",
      session: { ...hls, startPositionSeconds: -5 },
      expected: 0,
    },
    {
      name: "an HLS stream with an unusable start",
      session: { ...hls, startPositionSeconds: Number.NaN },
      expected: 0,
    },
  ])("is $expected for $name", ({ session, expected }) => {
    expect(mediaClockOffset(session as TimelineSession)).toBe(expected);
  });
});

describe("clampPosition", () => {
  it.each([
    { position: 50, expected: 50 },
    { position: 0, expected: 0 },
    { position: 100, expected: 100 },
    { position: 150, expected: 100 },
    { position: -10, expected: 0 },
    { position: Number.NaN, expected: 0 },
    { position: Number.POSITIVE_INFINITY, expected: 100 },
    { position: Number.NEGATIVE_INFINITY, expected: 0 },
  ])("clamps $position to $expected with a known duration", ({ position, expected }) => {
    expect(clampPosition(hls, position)).toBe(expected);
  });

  it.each([
    { position: 5000, expected: 5000 },
    { position: -1, expected: 0 },
    { position: Number.POSITIVE_INFINITY, expected: 0 },
  ])("clamps $position to $expected without a duration", ({ position, expected }) => {
    expect(clampPosition(hlsWithoutDuration, position)).toBe(expected);
  });
});

describe("displayedPosition", () => {
  it.each([
    { name: "HLS at its start", session: hls, currentTime: 0, expected: 30 },
    { name: "HLS while playing", session: hls, currentTime: 12.5, expected: 42.5 },
    { name: "HLS past the end", session: hls, currentTime: 90, expected: 100 },
    { name: "HLS before metadata", session: hls, currentTime: Number.NaN, expected: 30 },
    { name: "HLS with a negative time", session: hls, currentTime: -3, expected: 30 },
    {
      name: "HLS without a duration",
      session: hlsWithoutDuration,
      currentTime: 500,
      expected: 530,
    },
    { name: "a file", session: file, currentTime: 12.5, expected: 12.5 },
    { name: "a file past the end", session: file, currentTime: 120, expected: 100 },
    { name: "a file before metadata", session: file, currentTime: Number.NaN, expected: 0 },
  ])("shows $expected for $name", ({ session, currentTime, expected }) => {
    expect(displayedPosition(session, currentTime)).toBe(expected);
  });
});

describe("mediaTimeForPosition", () => {
  it.each([
    { name: "HLS inside the stream", session: hls, position: 42.5, expected: 12.5 },
    { name: "HLS at its start", session: hls, position: 30, expected: 0 },
    { name: "HLS before its start", session: hls, position: 10, expected: 0 },
    { name: "HLS past the end", session: hls, position: 150, expected: 70 },
    { name: "a file", session: file, position: 42.5, expected: 42.5 },
    { name: "a file before the start", session: file, position: -5, expected: 0 },
    { name: "an unusable position", session: file, position: Number.NaN, expected: 0 },
  ])("maps $name to $expected", ({ session, position, expected }) => {
    expect(mediaTimeForPosition(session, position)).toBe(expected);
  });

  it.each([0, 12.5, 69.9])("inverts displayedPosition for HLS time %d", (currentTime) => {
    expect(mediaTimeForPosition(hls, displayedPosition(hls, currentTime))).toBeCloseTo(currentTime);
  });
});

describe("displayedRanges", () => {
  it("shifts HLS ranges by the start and clamps them to the file", () => {
    expect(
      displayedRanges(hls, [
        { start: 0, end: 12 },
        { start: 50, end: 90 },
      ]),
    ).toEqual([
      { start: 30, end: 42 },
      { start: 80, end: 100 },
    ]);
  });

  it("keeps file ranges and drops ranges that end up empty", () => {
    expect(
      displayedRanges(file, [
        { start: 0, end: 0 },
        { start: 10, end: 20 },
        { start: 120, end: 150 },
      ]),
    ).toEqual([{ start: 10, end: 20 }]);
  });

  it("maps an open end to the end of the file", () => {
    expect(displayedRanges(hls, [{ start: 0, end: Number.POSITIVE_INFINITY }])).toEqual([
      { start: 30, end: 100 },
    ]);
  });
});
