import { describe, expect, it } from "vitest";

import { colors, contrastRatio, type ColorToken } from "./index";

// WCAG 2.x AA: normal text needs 4.5:1, large text and UI components (icons, badges,
// focus rings) need 3:1.
const textContrast = 4.5;
const nonTextContrast = 3;

interface ColorPair {
  readonly foreground: ColorToken;
  readonly background: ColorToken;
}

function pairs(
  foregrounds: readonly ColorToken[],
  backgrounds: readonly ColorToken[],
): ColorPair[] {
  return foregrounds.flatMap((foreground) =>
    backgrounds.map((background) => ({ foreground, background })),
  );
}

describe("colors", () => {
  it("encodes the ADR 0011 values exactly", () => {
    expect(colors).toMatchObject({
      background: "#0E1014",
      surface: "#181B21",
      surfaceRaised: "#22262E",
      divider: "#2C313A",
      text: "#E8EAED",
      textMuted: "#9AA0A6",
      accent: "#FF6B57",
      accentHover: "#FF8572",
      onAccent: "#3A0E07",
    });
  });

  it("uses six-digit hex for every token", () => {
    for (const value of Object.values(colors)) {
      expect(value).toMatch(/^#[0-9A-F]{6}$/);
    }
  });
});

describe("text contrast (>= 4.5:1)", () => {
  it.each([
    ...pairs(["text", "textMuted"], ["background", "surface", "surfaceRaised"]),
    ...pairs(["onAccent"], ["accent"]),
  ])("$foreground on $background", ({ foreground, background }) => {
    expect(contrastRatio(colors[foreground], colors[background])).toBeGreaterThanOrEqual(
      textContrast,
    );
  });
});

describe("non-text contrast (>= 3:1)", () => {
  it.each(pairs(["accent", "directPlay", "directStream", "transcode"], ["background", "surface"]))(
    "$foreground on $background",
    ({ foreground, background }) => {
      expect(contrastRatio(colors[foreground], colors[background])).toBeGreaterThanOrEqual(
        nonTextContrast,
      );
    },
  );
});
