// Class names the player's components share. Colours come from the ui-tokens theme only.

const iconButtonShape =
  "inline-flex shrink-0 items-center justify-center rounded-md hover:bg-surface-raised";

/** A square button with an icon and an accessible name. */
export const iconButtonClass = `size-10 text-text ${iconButtonShape}`;

/** The larger play/pause button in the centre of the control bar. */
export const largeIconButtonClass = `size-12 text-text ${iconButtonShape}`;

/** The large primary action, e.g. "Try again". */
export const primaryButtonClass =
  "rounded-md bg-accent px-4 py-2 font-medium text-on-accent hover:bg-accent-hover";

/** A secondary action next to or instead of the primary one. */
export const secondaryButtonClass =
  "rounded-md bg-surface-raised px-4 py-2 font-medium text-text hover:bg-divider";

/**
 * Fades the top bar and the control bar in and out. Hidden bars ignore the pointer but stay
 * focusable; any key press shows them again (useControlsVisibility). Without motion for users
 * who ask for reduced motion.
 */
export function fadeClass(visible: boolean): string {
  const state = visible ? "opacity-100" : "pointer-events-none opacity-0";
  return `transition-opacity duration-300 motion-reduce:transition-none ${state}`;
}

/** A toggle button: in the accent colour while pressed (repeat, shuffle). */
export function toggleButtonClass(pressed: boolean): string {
  return `size-10 ${pressed ? "text-accent" : "text-text"} ${iconButtonShape}`;
}

// The player covers the whole viewport, which reaches under a notch and the home indicator
// once the page opts into `viewport-fit=cover`; the bars keep their padding or the inset,
// whichever is larger, so no control sits under the hardware.
const safeInline =
  "pr-[max(calc(var(--spacing)*4),env(safe-area-inset-right))] pl-[max(calc(var(--spacing)*4),env(safe-area-inset-left))]";

/** Paddings of the top bar, clear of the safe-area insets. */
export const topBarPaddingClass = `${safeInline} pt-[max(calc(var(--spacing)*4),env(safe-area-inset-top))] pb-12`;

/** Paddings of the control bar, clear of the safe-area insets. */
export const controlBarPaddingClass = `${safeInline} pt-12 pb-[max(calc(var(--spacing)*4),env(safe-area-inset-bottom))]`;
