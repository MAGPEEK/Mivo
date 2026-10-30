import { describe, expect, it } from "vitest";

import type { TimelineSession } from "../timeline/timeline";
import type { TimeRange } from "../timeline/timeRanges";
import { planSeek, restartEndMarginSeconds, type SeekPlan, type SeekState } from "./planSeek";

const duration = 100;
const hls: TimelineSession = {
  delivery: "hls",
  startPositionSeconds: 30,
  durationSeconds: duration,
};
const file: TimelineSession = {
  delivery: "file",
  startPositionSeconds: 0,
  durationSeconds: duration,
};
const hlsWithoutDuration: TimelineSession = { delivery: "hls", startPositionSeconds: 30 };

// Element clock: produced segments cover 0–24 s (file 30–54 s), a buffered island 40–44 s
// (file 70–74 s) after a local seek that the seekable range has not caught up with yet.
const seekable: readonly TimeRange[] = [{ start: 0, end: 24 }];
const buffered: readonly TimeRange[] = [
  { start: 0, end: 8 },
  { start: 40, end: 44 },
];

function state(session: TimelineSession, ranges: Partial<SeekState> = {}): SeekState {
  return { session, seekable, buffered, ...ranges };
}

describe("planSeek", () => {
  it.each<{ name: string; target: number; expected: SeekPlan }>([
    { name: "inside the seekable range", target: 40, expected: { kind: "local", time: 10 } },
    { name: "at the session start", target: 30, expected: { kind: "local", time: 0 } },
    { name: "at the end of the produced range", target: 54, expected: { kind: "local", time: 24 } },
    { name: "inside a buffered range only", target: 72, expected: { kind: "local", time: 42 } },
    { name: "after the produced range", target: 60, expected: { kind: "restart", position: 60 } },
    { name: "before the session start", target: 10, expected: { kind: "restart", position: 10 } },
    { name: "before the file", target: -5, expected: { kind: "restart", position: 0 } },
    {
      name: "past the end of the file",
      target: 250,
      expected: { kind: "restart", position: duration - restartEndMarginSeconds },
    },
    {
      name: "at an unusable target",
      target: Number.NaN,
      expected: { kind: "restart", position: 0 },
    },
  ])("plans HLS seeks $name", ({ target, expected }) => {
    expect(planSeek(target, state(hls))).toEqual(expected);
  });

  it.each<{ name: string; target: number; expected: SeekPlan }>([
    { name: "inside the file", target: 60, expected: { kind: "local", time: 60 } },
    { name: "before the file", target: -5, expected: { kind: "local", time: 0 } },
    { name: "past the end", target: 250, expected: { kind: "local", time: duration } },
  ])("seeks direct play locally $name", ({ target, expected }) => {
    expect(planSeek(target, state(file, { seekable: [], buffered: [] }))).toEqual(expected);
  });

  it("restarts an HLS stream that has no ranges yet", () => {
    expect(planSeek(35, state(hls, { seekable: [], buffered: [] }))).toEqual({
      kind: "restart",
      position: 35,
    });
  });

  it("keeps a restart target as it is when the duration is unknown", () => {
    expect(planSeek(5000, state(hlsWithoutDuration))).toEqual({
      kind: "restart",
      position: 5000,
    });
  });

  it("restarts very short files at the beginning", () => {
    const shortClip: TimelineSession = { ...hls, startPositionSeconds: 0, durationSeconds: 0.5 };

    expect(planSeek(0.4, state(shortClip, { seekable: [], buffered: [] }))).toEqual({
      kind: "restart",
      position: 0,
    });
  });

  it("treats a delivery added later like a stream", () => {
    const future = { ...hls, delivery: "dash" } as unknown as TimelineSession;

    expect(planSeek(80, state(future))).toEqual({ kind: "restart", position: 80 });
  });
});
