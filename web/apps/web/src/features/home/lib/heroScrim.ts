// The hero text sits on `background` at the `textScrimOpacity` token (0.9), whose contrast over
// any image the ui-tokens tests check. Every `/90` below must match the token (checked by
// hero.test.ts); Tailwind needs literal class names, so they cannot be built from the number.

/**
 * Scrim layer over the backdrop from `sm` on: solid over the left half, which holds the text
 * column, fading out to the right. Below `sm` the text panel carries the scrim instead.
 */
export const heroScrimClass =
  "max-sm:hidden bg-linear-to-r from-background/90 via-background/90 via-50% to-transparent";

/**
 * Classes of the panel holding the hero text. Below `sm` the text spans the full width, so the
 * whole panel is solid scrim (a short gradient strip above it softens the edge, no text sits
 * there); from `sm` on the panel is clear and the scrim layer covers it.
 */
export const heroTextPanelClass =
  "relative w-full p-5 max-sm:bg-background/90 max-sm:before:absolute max-sm:before:inset-x-0 max-sm:before:bottom-full max-sm:before:h-12 max-sm:before:bg-linear-to-t max-sm:before:from-background/90 max-sm:before:to-transparent sm:p-8";

/** Width classes of the hero text column, kept inside the solid part of the scrim. */
export const heroTextColumnClass = "w-full max-w-xl sm:w-1/2";
