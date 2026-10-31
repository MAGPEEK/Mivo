import { describe, expect, it } from "vitest";

import { playerFileDurationSeconds, playerFileId, playerItem } from "../test/playerFixtures";
import { resolvePlaybackStart } from "./playbackStart";

const item = playerItem();

describe("resolvePlaybackStart", () => {
  it("plays the primary file from the beginning by default", () => {
    expect(resolvePlaybackStart(item, undefined, undefined)).toEqual({
      fileId: playerFileId,
      startPositionSeconds: 0,
    });
  });

  it.each([
    { startAt: 42.5, expected: 42.5 },
    { startAt: 1e21, expected: playerFileDurationSeconds - 1 },
    { startAt: playerFileDurationSeconds, expected: playerFileDurationSeconds - 1 },
    { startAt: -3, expected: 0 },
    { startAt: Number.NaN, expected: 0 },
    { startAt: Number.POSITIVE_INFINITY, expected: 0 },
  ])("clamps the start $startAt to $expected", ({ startAt, expected }) => {
    expect(resolvePlaybackStart(item, playerFileId, startAt)?.startPositionSeconds).toBe(expected);
  });

  it("starts at 0 when the file is shorter than the end margin", () => {
    const [file] = item.files;
    const shortItem = playerItem({ files: file ? [{ ...file, durationSeconds: 0.5 }] : [] });

    expect(resolvePlaybackStart(shortItem, undefined, 10)?.startPositionSeconds).toBe(0);
  });

  it.each([
    { name: "a file the item does not list", items: item, fileId: "file-9" },
    { name: "a file without known duration", items: unknownDurationItem(), fileId: playerFileId },
  ])("starts $name at 0, because the start cannot be checked", ({ items, fileId }) => {
    expect(resolvePlaybackStart(items, fileId, 30)).toEqual({ fileId, startPositionSeconds: 0 });
  });

  it("finds nothing to play in an item without files", () => {
    expect(resolvePlaybackStart(playerItem({ files: [] }), undefined, 5)).toBeUndefined();
  });
});

function unknownDurationItem() {
  return playerItem({
    files: item.files.map((file) => {
      const withoutDuration = { ...file };
      delete withoutDuration.durationSeconds;
      return withoutDuration;
    }),
  });
}
