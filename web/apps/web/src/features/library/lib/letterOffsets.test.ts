import { describe, expect, it } from "vitest";

import {
  alphaBarEntries,
  alphaBarLetters,
  columnLayout,
  focusTarget,
  itemsNeededFor,
  lettersQueryOf,
  rowCount,
  rowOfIndex,
  scrollTopOfOffset,
} from "./letterOffsets";

describe("alphaBarEntries", () => {
  it("lists # and A-Z, dimming the letters the server did not report", () => {
    const entries = alphaBarEntries({
      totalCount: 30,
      letters: [
        { letter: "#", offset: 0 },
        { letter: "B", offset: 4 },
      ],
    });

    expect(entries).toHaveLength(27);
    expect(entries.map((entry) => entry.letter)).toEqual(alphaBarLetters);
    expect(entries.find((entry) => entry.letter === "B")?.offset).toBe(4);
    expect(entries.find((entry) => entry.letter === "#")?.offset).toBe(0);
    expect(entries.find((entry) => entry.letter === "C")?.offset).toBeUndefined();
  });

  it("has no offsets before the letters arrived", () => {
    expect(alphaBarEntries(undefined).every((entry) => entry.offset === undefined)).toBe(true);
  });
});

describe("lettersQueryOf", () => {
  it.each([
    ["no sort is the title sort", {}, {}],
    ["explicit title sort drops the sort", { sort: "title" as const }, {}],
    [
      "keeps order and filters",
      { order: "desc" as const, genre: ["Drama"] },
      { order: "desc", genre: ["Drama"] },
    ],
    ["keeps the type", { type: "show" as const }, { type: "show" }],
  ])("%s", (_name, listQuery, expected) => {
    expect(lettersQueryOf(listQuery)).toEqual(expected);
  });

  it.each([
    ["another sort", { sort: "year" as const }],
    ["random sort", { sort: "random" as const, seed: 4 }],
    [
      "the unmatched filter, which the letters endpoint lacks",
      { matchState: "unmatched" as const },
    ],
  ])("is undefined for %s", (_name, listQuery) => {
    expect(lettersQueryOf(listQuery)).toBeUndefined();
  });
});

describe("columnLayout", () => {
  it.each([
    { width: 1000, min: 160, gap: 16, columns: 5, columnWidth: 187.2 },
    { width: 1000, min: 120, gap: 16, columns: 7, columnWidth: 129.14285714285714 },
    { width: 100, min: 160, gap: 16, columns: 1, columnWidth: 100 },
    { width: 0, min: 160, gap: 16, columns: 1, columnWidth: 0 },
    { width: 336, min: 160, gap: 16, columns: 2, columnWidth: 160 },
    { width: 335, min: 160, gap: 16, columns: 1, columnWidth: 335 },
  ])("width $width, min $min, gap $gap gives $columns columns", (row) => {
    const layout = columnLayout({
      containerWidth: row.width,
      minColumnWidth: row.min,
      gap: row.gap,
    });
    expect(layout.columns).toBe(row.columns);
    expect(layout.columnWidth).toBeCloseTo(row.columnWidth, 5);
  });
});

describe("rows", () => {
  it.each([
    [0, 4, 0],
    [1, 4, 1],
    [4, 4, 1],
    [5, 4, 2],
    [50_000, 7, 7143],
  ])("%d items in %d columns fill %d rows", (items, columns, rows) => {
    expect(rowCount(items, columns)).toBe(rows);
  });

  it.each([
    [0, 4, 0],
    [3, 4, 0],
    [4, 4, 1],
    [17, 5, 3],
  ])("position %d in %d columns sits in row %d", (index, columns, row) => {
    expect(rowOfIndex(index, columns)).toBe(row);
  });
});

describe("scrollTopOfOffset", () => {
  const base = { columns: 5, rowStride: 300, listTop: 200, clearance: 72 };

  it.each([
    [0, 128],
    [4, 128],
    [5, 428],
    [17, 1028],
    [49_999, 200 + 9999 * 300 - 72],
  ])("offset %d scrolls to %d", (offset, top) => {
    expect(scrollTopOfOffset({ ...base, offset })).toBe(top);
  });

  it("never scrolls above the document", () => {
    expect(scrollTopOfOffset({ ...base, listTop: 10, offset: 0 })).toBe(0);
  });
});

describe("itemsNeededFor", () => {
  it("needs the position plus one", () => {
    expect(itemsNeededFor(0)).toBe(1);
    expect(itemsNeededFor(119)).toBe(120);
  });
});

describe("focusTarget", () => {
  const base = { columns: 4, visibleRows: 3, itemCount: 30 };

  it.each([
    ["ArrowRight", 5, 6],
    ["ArrowLeft", 5, 4],
    ["ArrowDown", 5, 9],
    ["ArrowUp", 5, 1],
    ["PageDown", 5, 17],
    ["PageUp", 17, 5],
    ["ArrowLeft", 0, 0],
    ["ArrowUp", 2, 0],
    ["ArrowRight", 29, 29],
    ["ArrowDown", 28, 29],
    ["PageDown", 25, 29],
  ] as const)("%s from %d goes to %d", (key, index, target) => {
    expect(focusTarget({ ...base, key, index })).toBe(target);
  });

  it("ignores left and right in a single column", () => {
    expect(focusTarget({ ...base, columns: 1, key: "ArrowRight", index: 3 })).toBeUndefined();
    expect(focusTarget({ ...base, columns: 1, key: "ArrowDown", index: 3 })).toBe(4);
  });
});
