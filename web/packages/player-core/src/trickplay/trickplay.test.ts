import { describe, expect, it } from "vitest";

import {
  trickplayPollDelay,
  trickplayPollIntervalMilliseconds,
  trickplayPollMaxAttempts,
  trickplayPollMaxIntervalMilliseconds,
  trickplayTile,
  type TrickplayLayout,
} from "./trickplay";

// Three sheets of 10 × 10 tiles, one tile per 10 s: sheet 0 covers 0–1000 s.
const layout: TrickplayLayout = {
  tileWidth: 160,
  tileHeight: 90,
  columns: 10,
  rows: 10,
  intervalSeconds: 10,
  sheetUrls: ["/trickplay/0.jpg", "/trickplay/1.jpg", "/trickplay/2.jpg"],
};

describe("trickplayTile", () => {
  it.each<{ name: string; seconds: number; sheet: number; column: number; row: number }>([
    { name: "the start", seconds: 0, sheet: 0, column: 0, row: 0 },
    { name: "inside the first interval", seconds: 9.99, sheet: 0, column: 0, row: 0 },
    { name: "the second tile", seconds: 10, sheet: 0, column: 1, row: 0 },
    { name: "the end of a row", seconds: 95, sheet: 0, column: 9, row: 0 },
    { name: "the next row", seconds: 100, sheet: 0, column: 0, row: 1 },
    { name: "the last tile of a sheet", seconds: 999, sheet: 0, column: 9, row: 9 },
    { name: "the next sheet", seconds: 1000, sheet: 1, column: 0, row: 0 },
    { name: "in the last sheet", seconds: 2345, sheet: 2, column: 4, row: 3 },
    {
      name: "past the last sheet clamps to its last tile",
      seconds: 99_999,
      sheet: 2,
      column: 9,
      row: 9,
    },
    { name: "infinity clamps to the last tile", seconds: Infinity, sheet: 2, column: 9, row: 9 },
    { name: "a negative position clamps to the start", seconds: -5, sheet: 0, column: 0, row: 0 },
    { name: "an unknown position is the start", seconds: Number.NaN, sheet: 0, column: 0, row: 0 },
  ])("$name", ({ seconds, sheet, column, row }) => {
    expect(trickplayTile(layout, seconds)).toEqual({
      sheet,
      sheetUrl: layout.sheetUrls[sheet],
      column,
      row,
      x: column * layout.tileWidth,
      y: row * layout.tileHeight,
      width: layout.tileWidth,
      height: layout.tileHeight,
    });
  });

  it("clamps the sheet to the listed URLs when the manifest promises more", () => {
    const manifest = { ...layout, sheetCount: 6, sheetUrls: ["/trickplay/0.jpg"] };
    expect(trickplayTile(manifest, 2345)).toMatchObject({
      sheet: 0,
      sheetUrl: "/trickplay/0.jpg",
      column: 9,
      row: 9,
    });
  });

  it("returns the URL unchanged", () => {
    const opaque = "../sheets/a%20b.jpg?token=x#y";
    expect(trickplayTile({ ...layout, sheetUrls: [opaque] }, 0)?.sheetUrl).toBe(opaque);
  });

  it.each<{ name: string; layout: TrickplayLayout }>([
    { name: "no sheet URLs", layout: { ...layout, sheetUrls: [] } },
    { name: "an empty sheet URL", layout: { ...layout, sheetUrls: [""] } },
    { name: "no columns", layout: { ...layout, columns: 0 } },
    { name: "fractional rows", layout: { ...layout, rows: 2.5 } },
    { name: "no tile width", layout: { ...layout, tileWidth: 0 } },
    { name: "a negative tile height", layout: { ...layout, tileHeight: -90 } },
    { name: "no interval", layout: { ...layout, intervalSeconds: 0 } },
    { name: "an infinite interval", layout: { ...layout, intervalSeconds: Infinity } },
  ])("has no preview with $name", ({ layout: unusable }) => {
    expect(trickplayTile(unusable, 50)).toBeUndefined();
  });
});

describe("trickplayPollDelay", () => {
  it.each<[number, number | undefined]>([
    [0, trickplayPollIntervalMilliseconds],
    [1, trickplayPollIntervalMilliseconds],
    [2, trickplayPollIntervalMilliseconds * 2],
    [3, trickplayPollIntervalMilliseconds * 4],
    [4, trickplayPollMaxIntervalMilliseconds],
    [trickplayPollMaxAttempts - 1, trickplayPollMaxIntervalMilliseconds],
    [trickplayPollMaxAttempts, undefined],
    [trickplayPollMaxAttempts + 10, undefined],
    [Number.NaN, undefined],
    [Infinity, undefined],
  ])("after %s failed attempts waits %s ms", (failedAttempts, expected) => {
    expect(trickplayPollDelay(failedAttempts)).toBe(expected);
  });

  it("ends, with a bounded total wait", () => {
    let total = 0;
    let attempts = 1;
    for (let delay = trickplayPollDelay(attempts); delay !== undefined;) {
      total += delay;
      attempts += 1;
      delay = trickplayPollDelay(attempts);
    }
    expect(attempts).toBe(trickplayPollMaxAttempts);
    expect(total).toBeLessThanOrEqual(
      trickplayPollMaxAttempts * trickplayPollMaxIntervalMilliseconds,
    );
  });
});
