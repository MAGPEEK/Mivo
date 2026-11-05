import { describe, expect, it } from "vitest";

import {
  cardStride,
  cardWidths,
  pageScrollTarget,
  rowGap,
  rowCardSizes,
  rowKeyTarget,
  rowScrollEdges,
  shouldVirtualize,
  withActiveIndex,
} from "./rowScroll";

describe("rowScrollEdges", () => {
  it.each([
    { left: 0, client: 500, scroll: 1500, expected: { atStart: true, atEnd: false } },
    { left: 400, client: 500, scroll: 1500, expected: { atStart: false, atEnd: false } },
    { left: 1000, client: 500, scroll: 1500, expected: { atStart: false, atEnd: true } },
    // Fractional scroll positions on zoomed or high-DPI screens stop just short of the end.
    { left: 999.5, client: 500, scroll: 1500, expected: { atStart: false, atEnd: true } },
    { left: 0.5, client: 500, scroll: 1500, expected: { atStart: true, atEnd: false } },
    { left: 0, client: 500, scroll: 400, expected: { atStart: true, atEnd: true } },
    { left: 0, client: 0, scroll: 0, expected: { atStart: true, atEnd: true } },
  ])("reads $left of $scroll in $client as $expected", ({ left, client, scroll, expected }) => {
    expect(rowScrollEdges({ scrollLeft: left, clientWidth: client, scrollWidth: scroll })).toEqual(
      expected,
    );
  });
});

describe("pageScrollTarget", () => {
  const stride = 176;
  it.each([
    // 1000 px show five whole cards: a page moves by five strides.
    { left: 0, direction: 1, expected: 880 },
    { left: 880, direction: 1, expected: 1760 },
    { left: 1760, direction: -1, expected: 880 },
    // Clamped at both ends.
    { left: 2500, direction: 1, expected: 3000 },
    { left: 300, direction: -1, expected: 0 },
  ] as const)("moves from $left in direction $direction to $expected", (row) => {
    const geometry = { scrollLeft: row.left, clientWidth: 1000, scrollWidth: 4000 };
    expect(pageScrollTarget(geometry, row.direction, stride)).toBe(row.expected);
  });

  it("moves at least one card when the row is narrower than a card", () => {
    const geometry = { scrollLeft: 0, clientWidth: 100, scrollWidth: 4000 };
    expect(pageScrollTarget(geometry, 1, stride)).toBe(stride);
  });

  it("keeps the position of a row that does not scroll", () => {
    const geometry = { scrollLeft: 0, clientWidth: 1000, scrollWidth: 800 };
    expect(pageScrollTarget(geometry, 1, stride)).toBe(0);
  });
});

describe("rowKeyTarget", () => {
  it.each([
    { key: "ArrowRight", current: 0, expected: 1 },
    { key: "ArrowLeft", current: 2, expected: 1 },
    // Rows stop at their ends instead of wrapping, like the disabled arrow buttons.
    { key: "ArrowRight", current: 9, expected: undefined },
    { key: "ArrowLeft", current: 0, expected: undefined },
    { key: "Home", current: 5, expected: 0 },
    { key: "End", current: 5, expected: 9 },
    { key: "Home", current: 0, expected: undefined },
    { key: "End", current: 9, expected: undefined },
    { key: "ArrowDown", current: 3, expected: undefined },
    { key: "Enter", current: 3, expected: undefined },
  ])("moves $key from $current to $expected", ({ key, current, expected }) => {
    expect(rowKeyTarget(key, current, 10)).toBe(expected);
  });

  it("ignores keys in an empty row", () => {
    expect(rowKeyTarget("End", 0, 0)).toBeUndefined();
  });
});

describe("cardStride", () => {
  it.each([
    { format: "poster", expected: cardWidths.poster + rowGap },
    { format: "episode", expected: cardWidths.episode + rowGap },
    { format: "square", expected: cardWidths.square + rowGap },
  ] as const)("adds the gap to a $format card", ({ format, expected }) => {
    expect(cardStride(format)).toBe(expected);
  });

  it("makes episode cards wider than posters", () => {
    expect(cardWidths.episode).toBeGreaterThan(cardWidths.poster);
  });

  it("gives row images their fixed card width as sizes", () => {
    expect(rowCardSizes("poster")).toBe("160px");
    expect(rowCardSizes("episode")).toBe("256px");
  });
});

describe("shouldVirtualize", () => {
  it.each([
    { count: 0, expected: false },
    { count: 5, expected: false },
    { count: 24, expected: false },
    { count: 25, expected: true },
    { count: 500, expected: true },
  ])("virtualises $count cards: $expected", ({ count, expected }) => {
    expect(shouldVirtualize(count)).toBe(expected);
  });
});

describe("withActiveIndex", () => {
  it.each([
    { indexes: [3, 4, 5], active: 4, expected: [3, 4, 5] },
    { indexes: [3, 4, 5], active: 0, expected: [0, 3, 4, 5] },
    { indexes: [3, 4, 5], active: 9, expected: [3, 4, 5, 9] },
    { indexes: [], active: 2, expected: [2] },
  ])("keeps card $active in $indexes", ({ indexes, active, expected }) => {
    expect(withActiveIndex(indexes, active)).toEqual(expected);
  });
});
