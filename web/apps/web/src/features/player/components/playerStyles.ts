// Class names the player's components share. Colours come from the ui-tokens theme only.

/** A square button with an icon and an accessible name. */
export const iconButtonClass =
  "inline-flex size-10 shrink-0 items-center justify-center rounded-md text-text hover:bg-surface-raised";

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
