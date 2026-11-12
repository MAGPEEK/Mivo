import { describe, expect, it } from "vitest";

import { bubbleLeft, positionAtPointer, tileStyle } from "./seekPreview";

describe("positionAtPointer", () => {
  const box = { left: 100, width: 400 };

  it.each([
    [300, 50],
    [100, 0],
    [500, 100],
    [0, 0],
    [900, 100],
  ])("maps x=%i to %d s", (clientX, expected) => {
    expect(positionAtPointer(clientX, box, 100)).toBe(expected);
  });

  it("is 0 without a duration or a box", () => {
    expect(positionAtPointer(300, box, undefined)).toBe(0);
    expect(positionAtPointer(300, { left: 0, width: 0 }, 100)).toBe(0);
  });
});

describe("tileStyle", () => {
  it("crops the tile and scales the sheet to the display width", () => {
    const style = tileStyle(
      { sheetUrl: "http://x/0.jpg", x: 320, y: 90, tileWidth: 160, tileHeight: 90, columns: 10 },
      80,
    );

    expect(style).toMatchObject({
      width: "80px",
      height: "45px",
      backgroundImage: 'url("http://x/0.jpg")',
      backgroundSize: "800px auto",
      backgroundPosition: "-160px -45px",
    });
  });
});

describe("bubbleLeft", () => {
  it("clamps to both ends of the bar", () => {
    expect(bubbleLeft(50, 80)).toBe("clamp(80px, 50%, calc(100% - 80px))");
  });
});
