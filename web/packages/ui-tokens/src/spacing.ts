/** Base unit of the spacing scale in CSS pixels. Tailwind's `--spacing` uses the same unit. */
export const spacingUnit = 4;

/** Steps shared by all spacing scales. */
export type SpacingStep = "none" | "xxs" | "xs" | "sm" | "md" | "lg" | "xl" | "2xl" | "3xl" | "4xl";

/** A complete spacing scale in CSS pixels. */
export type SpacingScale = Readonly<Record<SpacingStep, number>>;

/**
 * Spacing scales: `web` is dense like the poster rows of the design, `tenFoot` is larger
 * because TV focus targets and gaps must be visible from a distance.
 */
export const spacing = {
  web: {
    none: 0,
    xxs: 2,
    xs: 4,
    sm: 8,
    md: 12,
    lg: 16,
    xl: 24,
    "2xl": 32,
    "3xl": 48,
    "4xl": 64,
  },
  tenFoot: {
    none: 0,
    xxs: 4,
    xs: 8,
    sm: 12,
    md: 16,
    lg: 24,
    xl: 32,
    "2xl": 48,
    "3xl": 64,
    "4xl": 96,
  },
} as const satisfies Readonly<Record<string, SpacingScale>>;

/**
 * Title-safe inset of a 1920x1080 TV screen in CSS pixels (5 % per edge), because many TVs
 * crop the outer area (overscan).
 */
export const tenFootSafeArea = {
  horizontal: 96,
  vertical: 54,
} as const;
