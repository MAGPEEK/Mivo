import { describe, expect, it } from "vitest";

import { contrastRatio, InvalidColorError, relativeLuminance } from "./index";

const maximumContrast = 21;
const minimumContrast = 1;
const precisionDigits = 2;

describe("relativeLuminance", () => {
  it.each([
    { color: "#000000", expected: 0 },
    { color: "#FFFFFF", expected: 1 },
    // A channel of 10/255 lies below the sRGB threshold and uses the linear segment.
    { color: "#0A0A0A", expected: 10 / 255 / 12.92 },
  ])("returns $expected for $color", ({ color, expected }) => {
    expect(relativeLuminance(color)).toBeCloseTo(expected, 6);
  });
});

describe("contrastRatio", () => {
  it("returns 21 for black on white", () => {
    expect(contrastRatio("#000000", "#FFFFFF")).toBeCloseTo(maximumContrast, precisionDigits);
  });

  it("returns 1 for identical colours", () => {
    expect(contrastRatio("#FF6B57", "#FF6B57")).toBe(minimumContrast);
    expect(contrastRatio("#000000", "#000000")).toBe(minimumContrast);
  });

  it("does not depend on the order of the colours", () => {
    expect(contrastRatio("#FFFFFF", "#000000")).toBe(contrastRatio("#000000", "#FFFFFF"));
  });

  it("treats 3-digit hex like its 6-digit expansion", () => {
    expect(contrastRatio("#000", "#fff")).toBe(contrastRatio("#000000", "#FFFFFF"));
    expect(contrastRatio("#f63", "#123")).toBe(contrastRatio("#FF6633", "#112233"));
  });

  it("ignores the case of hex digits", () => {
    expect(contrastRatio("#ff6b57", "#0e1014")).toBe(contrastRatio("#FF6B57", "#0E1014"));
  });

  it("matches a known reference value", () => {
    // #777777 on white is the classic borderline grey: 4.48:1.
    expect(contrastRatio("#777777", "#FFFFFF")).toBeCloseTo(4.48, precisionDigits);
  });

  it.each(["", "000000", "#00000", "#0000", "#00000000", "#GGGGGG", "red", " #000000", "#000 "])(
    'throws InvalidColorError for "%s"',
    (invalid) => {
      expect(() => contrastRatio(invalid, "#000000")).toThrow(InvalidColorError);
      expect(() => contrastRatio("#000000", invalid)).toThrow(InvalidColorError);
    },
  );

  it("reports the rejected value on the error", () => {
    const error: unknown = (() => {
      try {
        contrastRatio("#12", "#000000");
        return undefined;
      } catch (caught: unknown) {
        return caught;
      }
    })();

    expect(error).toBeInstanceOf(InvalidColorError);
    expect(error).toMatchObject({ name: "InvalidColorError", value: "#12" });
  });
});
