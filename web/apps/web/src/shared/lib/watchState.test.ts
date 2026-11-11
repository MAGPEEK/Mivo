import type { ViewerState } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  canMarkWatched,
  expectedViewerState,
  noWatchBadges,
  resumeSeconds,
  watchBadges,
} from "./watchState";

function state(fields: Partial<ViewerState> = {}): ViewerState {
  return { watched: false, positionSeconds: 0, playCount: 0, ...fields };
}

describe("canMarkWatched", () => {
  it.each([
    ["movie", true],
    ["episode", true],
    ["show", true],
    ["season", true],
    ["audiobook", false],
    ["extra", false],
    ["album", false],
    ["track", false],
    ["photo", false],
  ])("%s: %s", (kind, expected) => {
    expect(canMarkWatched(kind)).toBe(expected);
  });
});

describe("resumeSeconds", () => {
  it.each([
    [undefined, undefined],
    [state(), undefined],
    [state({ positionSeconds: 2530.5 }), 2530.5],
    [state({ positionSeconds: Number.NaN }), undefined],
    [state({ positionSeconds: -4 }), undefined],
  ])("%o gives %s", (viewerState, expected) => {
    expect(resumeSeconds(viewerState)).toBe(expected);
  });
});

describe("watchBadges", () => {
  it("shows nothing without a viewer state, whatever the kind", () => {
    for (const kind of ["movie", "episode", "show", "season", "album", "photo"]) {
      expect(watchBadges({ kind, durationSeconds: 600 })).toBe(noWatchBadges);
    }
  });

  it("shows the watched corner of a movie", () => {
    const badges = watchBadges({ kind: "movie", viewerState: state({ watched: true }) });

    expect(badges).toEqual({
      watched: true,
      progressPercent: undefined,
      unwatchedCount: undefined,
    });
  });

  it.each([
    [150, 600, 25],
    [1, 6000, 1],
    [599.9, 600, 100],
    [900, 600, 100],
  ])("turns position %d of %d seconds into %d percent", (positionSeconds, duration, expected) => {
    const badges = watchBadges({
      kind: "episode",
      viewerState: state({ positionSeconds }),
      durationSeconds: duration,
    });

    expect(badges.progressPercent).toBe(expected);
  });

  it.each([undefined, 0, Number.NaN])("shows no progress with duration %s", (durationSeconds) => {
    const badges = watchBadges({
      kind: "movie",
      viewerState: state({ positionSeconds: 30 }),
      durationSeconds,
    });

    expect(badges.progressPercent).toBeUndefined();
  });

  it("keeps the corner and the progress of a watched item that is played again", () => {
    const badges = watchBadges({
      kind: "movie",
      viewerState: state({ watched: true, positionSeconds: 300 }),
      durationSeconds: 600,
    });

    expect(badges).toEqual({ watched: true, progressPercent: 50, unwatchedCount: undefined });
  });

  it.each(["show", "season"])("counts the unwatched episodes of a %s", (kind) => {
    const badges = watchBadges({ kind, viewerState: state({ unwatchedCount: 4 }) });

    expect(badges).toEqual({ watched: false, progressPercent: undefined, unwatchedCount: 4 });
  });

  it("shows the corner instead of a count on a watched show", () => {
    const viewerState = state({ watched: true, unwatchedCount: 0 });

    expect(watchBadges({ kind: "show", viewerState })).toEqual({
      watched: true,
      progressPercent: undefined,
      unwatchedCount: undefined,
    });
  });

  it("ignores an unwatched count on other kinds", () => {
    const badges = watchBadges({ kind: "movie", viewerState: state({ unwatchedCount: 3 }) });

    expect(badges.unwatchedCount).toBeUndefined();
  });
});

describe("expectedViewerState", () => {
  it("marks an item without state watched", () => {
    expect(expectedViewerState(undefined, true)).toEqual(state({ watched: true }));
  });

  it("clears the resume position and keeps the history", () => {
    const current = state({
      positionSeconds: 120,
      playCount: 2,
      lastPlayedAt: "2026-11-05T20:14:03Z",
    });

    expect(expectedViewerState(current, false)).toEqual({
      watched: false,
      positionSeconds: 0,
      playCount: 2,
      lastPlayedAt: "2026-11-05T20:14:03Z",
    });
  });

  it("sets the unwatched count of a watched show to 0 and drops it when unwatched", () => {
    const show = state({ unwatchedCount: 4 });

    expect(expectedViewerState(show, true).unwatchedCount).toBe(0);
    expect("unwatchedCount" in expectedViewerState(show, false)).toBe(false);
  });
});
