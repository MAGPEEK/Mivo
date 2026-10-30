import { useSyncExternalStore } from "react";

/**
 * Screens narrower than Tailwind's `md` breakpoint, written like Tailwind's own `max-md`
 * variant, so script and styles switch at exactly the same width.
 */
export const narrowScreenQuery = "(width < 48rem)";

// The DOM types promise matchMedia, but jsdom leaves it undefined; such environments count
// as wide screens.
type MaybeMatchMedia = Partial<Pick<Window, "matchMedia">>;

function mediaQueryList(): MediaQueryList | undefined {
  return (window as MaybeMatchMedia).matchMedia?.(narrowScreenQuery);
}

function subscribe(onChange: () => void): () => void {
  const list = mediaQueryList();
  list?.addEventListener("change", onChange);
  return () => {
    list?.removeEventListener("change", onChange);
  };
}

function isNarrow(): boolean {
  return mediaQueryList()?.matches ?? false;
}

/** Whether the window is narrower than the `md` breakpoint; follows resizing and rotation. */
export function useNarrowScreen(): boolean {
  return useSyncExternalStore(subscribe, isNarrow);
}
