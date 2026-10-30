import { describe, expect, it } from "vitest";

import {
  fontStackSans,
  fontWeights,
  minimumTenFootFontSize,
  radii,
  spacing,
  spacingUnit,
  tenFootSafeArea,
  typeScale,
  type SpacingStep,
  type TypeScaleStep,
  type TypeStyle,
} from "./index";

const minimumRadius = 6;
const maximumRadius = 10;

describe("typography", () => {
  it("puts Inter first in the font stack", () => {
    expect(fontStackSans.split(",")[0]).toBe("Inter");
  });

  it("orders the font weights from light to heavy", () => {
    const weights = Object.values(fontWeights);
    expect(weights).toEqual([...weights].sort((left, right) => left - right));
  });

  it.each(Object.entries(typeScale.tenFoot))(
    "keeps ten-foot step %s at or above the minimum size",
    (_step, style) => {
      expect(style.fontSize).toBeGreaterThanOrEqual(minimumTenFootFontSize);
    },
  );

  it.each(Object.keys(typeScale.web) as TypeScaleStep[])(
    "makes ten-foot step %s at least as large as the web step",
    (step) => {
      expect(typeScale.tenFoot[step].fontSize).toBeGreaterThanOrEqual(typeScale.web[step].fontSize);
    },
  );

  it.each([...Object.entries(typeScale.web), ...Object.entries(typeScale.tenFoot)])(
    "gives step %s a line height no smaller than its font size",
    (_step, style) => {
      expect(style.lineHeight).toBeGreaterThanOrEqual(style.fontSize);
    },
  );

  it.each(["web", "tenFoot"] as const)("grows strictly along the %s scale", (scale) => {
    const sizes = Object.values<TypeStyle>(typeScale[scale]).map((style) => style.fontSize);
    const sortedUnique = [...new Set(sizes)].sort((left, right) => left - right);

    expect(sizes).toEqual(sortedUnique);
  });
});

describe("spacing", () => {
  it.each(Object.keys(spacing.web) as SpacingStep[])(
    "makes ten-foot step %s at least as large as the web step",
    (step) => {
      expect(spacing.tenFoot[step]).toBeGreaterThanOrEqual(spacing.web[step]);
    },
  );

  it("builds the web scale from half spacing units so Tailwind classes can express it", () => {
    const halfUnit = spacingUnit / 2;
    for (const pixels of Object.values(spacing.web)) {
      expect(pixels % halfUnit).toBe(0);
    }
  });

  it("insets the ten-foot safe area on both axes", () => {
    expect(tenFootSafeArea.horizontal).toBeGreaterThan(0);
    expect(tenFootSafeArea.vertical).toBeGreaterThan(0);
  });
});

describe("radii", () => {
  it.each(Object.entries(radii))("keeps %s within 6 to 10 px", (_name, pixels) => {
    expect(pixels).toBeGreaterThanOrEqual(minimumRadius);
    expect(pixels).toBeLessThanOrEqual(maximumRadius);
  });
});
