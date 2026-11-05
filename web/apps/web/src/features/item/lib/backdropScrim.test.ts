import { colors, contrastRatio, type HexColor } from "@mivo/ui-tokens";
import { describe, expect, it } from "vitest";

import {
  backdropBrightnessPercent,
  backdropImageClass,
  backdropScrimClass,
  backdropScrimOpacityPercent,
} from "./backdropScrim";

const wcagAaNormalText = 4.5;
const percent = 100;
const maxChannel = 255;
const hexRadix = 16;

// sRGB encoding of a linear-light value (IEC 61966-2-1).
const linearThreshold = 0.0031308;
const linearSlope = 12.92;
const gammaScale = 1.055;
const gammaOffset = 0.055;
const gammaExponent = 2.4;

function encodeSrgb(linear: number): number {
  return linear <= linearThreshold
    ? linear * linearSlope
    : gammaScale * linear ** (1 / gammaExponent) - gammaOffset;
}

// Browsers may apply `brightness()` to gamma-encoded values or to linear light; linear light
// gives the brighter result, so the check takes whichever is brighter.
function dimmedWhiteChannel(brightness: number): number {
  return Math.max(brightness, encodeSrgb(brightness)) * maxChannel;
}

// The brightest colour text can sit on: a white image, dimmed, under the scrim token.
function worstCaseBackground(): HexColor {
  const dimmed = dimmedWhiteChannel(backdropBrightnessPercent / percent);
  const scrimChannel = Number.parseInt(colors.scrim.slice(1, 3), hexRadix);
  const opacity = backdropScrimOpacityPercent / percent;
  const channel = Math.round(dimmed * (1 - opacity) + scrimChannel * opacity);
  const hex = channel.toString(hexRadix).padStart(2, "0");
  return `#${hex}${hex}${hex}`;
}

describe("backdrop scrim", () => {
  it("uses the documented brightness and scrim token opacity", () => {
    expect(backdropImageClass.split(" ")).toContain(
      `brightness-${String(backdropBrightnessPercent)}`,
    );
    expect(backdropScrimClass).toBe(`bg-scrim/${String(backdropScrimOpacityPercent)}`);
  });

  it.each([
    { token: "text", color: colors.text },
    { token: "textMuted", color: colors.textMuted },
    { token: "accent", color: colors.accent },
  ])("keeps $token at AA contrast over a white image", ({ color }) => {
    expect(contrastRatio(color, worstCaseBackground())).toBeGreaterThanOrEqual(wcagAaNormalText);
  });
});
