/**
 * Family name of the self-hosted variable Inter. The apps bundle it through
 * `@fontsource-variable/inter` (ADR 0010), which registers the font under this name.
 */
export const selfHostedSansFamily = "Inter Variable";

/**
 * Font stack with the self-hosted Inter first. A locally installed Inter and the system fonts
 * only bridge the time until it has loaded.
 */
export const fontStackSans = `"${selfHostedSansFamily}", Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`;

/** Monospace stack for technical details such as codec chips and the stream info overlay. */
export const fontStackMono =
  'ui-monospace, "Cascadia Mono", "SF Mono", Menlo, Consolas, "Liberation Mono", monospace';

/** Font weights used by the UI. Inter ships all of them as one variable font. */
export const fontWeights = {
  regular: 400,
  medium: 500,
  semibold: 600,
  bold: 700,
} as const;

/** Size and line height of one step of a type scale, both in CSS pixels. */
export interface TypeStyle {
  readonly fontSize: number;
  readonly lineHeight: number;
}

/** Steps shared by all type scales, so a component can swap the scale without renaming. */
export type TypeScaleStep = "xs" | "sm" | "base" | "lg" | "xl" | "2xl" | "3xl" | "4xl" | "5xl";

/** A complete type scale. */
export type TypeScale = Readonly<Record<TypeScaleStep, TypeStyle>>;

/** Smallest font size that stays readable from the sofa (10-foot UI). */
export const minimumTenFootFontSize = 20;

/** Type scales: `web` for desktop and phone browsers, `tenFoot` for TVs viewed from a distance. */
export const typeScale = {
  web: {
    xs: { fontSize: 12, lineHeight: 16 },
    sm: { fontSize: 14, lineHeight: 20 },
    base: { fontSize: 16, lineHeight: 24 },
    lg: { fontSize: 18, lineHeight: 28 },
    xl: { fontSize: 20, lineHeight: 28 },
    "2xl": { fontSize: 24, lineHeight: 32 },
    "3xl": { fontSize: 30, lineHeight: 36 },
    "4xl": { fontSize: 36, lineHeight: 40 },
    "5xl": { fontSize: 48, lineHeight: 52 },
  },
  tenFoot: {
    xs: { fontSize: 20, lineHeight: 28 },
    sm: { fontSize: 22, lineHeight: 30 },
    base: { fontSize: 24, lineHeight: 32 },
    lg: { fontSize: 28, lineHeight: 36 },
    xl: { fontSize: 32, lineHeight: 40 },
    "2xl": { fontSize: 36, lineHeight: 44 },
    "3xl": { fontSize: 44, lineHeight: 52 },
    "4xl": { fontSize: 56, lineHeight: 64 },
    "5xl": { fontSize: 72, lineHeight: 80 },
  },
} as const satisfies Readonly<Record<string, TypeScale>>;
