// How the detail backdrop is darkened so that text on it keeps WCAG AA contrast over any
// artwork (DT-01). The numbers and the Tailwind classes describe the same treatment; the
// classes stay literal so Tailwind finds them, and backdropScrim.test.ts checks that they
// match the numbers and that the worst case (a white image) still passes AA.

/** Brightness of the blurred image in percent (CSS `brightness()`). */
export const backdropBrightnessPercent = 50;

/** Opacity in percent of the scrim token laid over the dimmed image. */
export const backdropScrimOpacityPercent = 75;

/** Classes of the backdrop image: blurred, dimmed and scaled so the blur has no soft edge. */
export const backdropImageClass = "size-full scale-110 object-cover blur-2xl brightness-50";

/** Classes of the scrim over the image, using the `scrim` colour token. */
export const backdropScrimClass = "bg-scrim/75";

/** Fades the backdrop into the page background at the bottom, as in the prototype. */
export const backdropFadeClass = "bg-linear-to-b from-transparent via-transparent to-background";

/** Plain background when the item has no artwork at all: no blur, no image. */
export const plainBackdropClass = "bg-linear-to-br from-surface-raised via-surface to-background";
