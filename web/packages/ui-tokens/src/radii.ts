/** Corner radii in CSS pixels (ADR 0011: 6–10 px). */
export const radii = {
  /** Chips, badges and small controls. */
  sm: 6,
  /** Buttons, inputs and poster cards. */
  md: 8,
  /** Dialogs, menus and the hero area. */
  lg: 10,
} as const;

/** Name of a radius token. */
export type RadiusToken = keyof typeof radii;
