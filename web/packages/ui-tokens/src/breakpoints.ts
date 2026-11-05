/**
 * Viewport widths in CSS pixels at which the web layout changes. They equal Tailwind's `sm` and
 * `md` defaults; theme.css declares them, so the `sm:`/`max-sm:` variants and the script below
 * switch at exactly the same width. Below `sm` is a phone (bottom navigation), below `md` the
 * sidebar shrinks to a rail.
 */
export const breakpoints = {
  sm: 640,
  md: 768,
} as const satisfies Readonly<Record<string, number>>;

/** Name of a layout breakpoint. */
export type BreakpointToken = keyof typeof breakpoints;

// CSS reference size of 1rem; Tailwind writes its breakpoints in rem, so do the queries.
const pixelsPerRem = 16;

/**
 * Media query matching viewports narrower than `breakpoint`, written like Tailwind's `max-*`
 * variants, e.g. `(width < 40rem)` for `sm`.
 */
export function belowBreakpointQuery(breakpoint: BreakpointToken): string {
  return `(width < ${String(breakpoints[breakpoint] / pixelsPerRem)}rem)`;
}
