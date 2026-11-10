import type { ItemDetail } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { playerFileId, playerItem } from "../test/playerFixtures";
import { progressTarget } from "./progressTarget";

function bookOfTwoParts(): ItemDetail {
  const [file] = playerItem().files;
  if (file === undefined) {
    throw new Error("the fixture item has no file");
  }
  return playerItem({
    kind: "audiobook",
    files: [
      { ...file, id: "part-1", durationSeconds: 600 },
      { ...file, id: "part-2", durationSeconds: 300 },
    ],
    chapters: [
      { title: "One", startMs: 0, endMs: 600_000, fileId: "part-1" },
      { title: "Two", startMs: 0, endMs: 300_000, fileId: "part-2" },
    ],
  });
}

describe("progressTarget", () => {
  it.each(["movie", "episode"] as const)("reports the file position of a %s", (kind) => {
    expect(progressTarget(playerItem({ kind }), playerFileId)).toEqual({ offsetSeconds: 0 });
  });

  it.each(["track", "photo"] as const)("does not report a %s", (kind) => {
    expect(progressTarget(playerItem({ kind }), playerFileId)).toBeUndefined();
  });

  it("reports a single-file audiobook without chapters from 0", () => {
    expect(progressTarget(playerItem({ kind: "audiobook" }), playerFileId)).toEqual({
      offsetSeconds: 0,
    });
  });

  it("adds the parts before the played one for an audiobook of several parts", () => {
    expect(progressTarget(bookOfTwoParts(), "part-1")).toEqual({ offsetSeconds: 0 });
    expect(progressTarget(bookOfTwoParts(), "part-2")).toEqual({ offsetSeconds: 600 });
  });

  it("does not report an audiobook file the item does not list", () => {
    expect(progressTarget(bookOfTwoParts(), "elsewhere")).toBeUndefined();
  });
});
