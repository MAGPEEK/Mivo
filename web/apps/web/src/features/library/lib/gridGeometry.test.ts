import { describe, expect, it } from "vitest";

import { gridGeometryOf } from "./gridGeometry";
import { columnLayout } from "./letterOffsets";

describe("gridGeometryOf", () => {
  it("follows the size slider for the grid", () => {
    expect(gridGeometryOf({ viewStyle: "grid", cardStep: 0 }).minColumnWidth).toBe(120);
    expect(gridGeometryOf({ viewStyle: "grid", cardStep: 4 }).minColumnWidth).toBe(230);
  });

  it("keeps the default card size without a view", () => {
    expect(gridGeometryOf(undefined).minColumnWidth).toBe(160);
  });

  it.each([
    ["poster", 240 + 56],
    ["square", 160 + 56],
    ["episode", 90 + 56],
  ] as const)("a %s card of 160 px is %d px high", (format, height) => {
    expect(gridGeometryOf(undefined, format).rowHeightOf(160)).toBe(height);
  });

  it("widens the columns of episode cards", () => {
    expect(gridGeometryOf({ viewStyle: "grid", cardStep: 2 }, "episode").minColumnWidth).toBe(256);
  });

  it.each([
    ["list", 44],
    ["details", 129],
  ] as const)("lays %s out as one column of %d px rows", (viewStyle, height) => {
    const geometry = gridGeometryOf({ viewStyle, cardStep: 2 });
    const layout = columnLayout({
      containerWidth: 1400,
      minColumnWidth: geometry.minColumnWidth,
      gap: geometry.columnGap,
    });

    expect(layout).toEqual({ columns: 1, columnWidth: 1400 });
    expect(geometry.rowHeightOf(1400)).toBe(height);
    expect(geometry.rowGap).toBe(0);
  });
});
