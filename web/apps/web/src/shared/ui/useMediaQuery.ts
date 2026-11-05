import { useCallback, useSyncExternalStore } from "react";

// The DOM types promise matchMedia, but jsdom leaves it undefined; such environments count
// as not matching.
type MaybeMatchMedia = Partial<Pick<Window, "matchMedia">>;

function mediaQueryList(query: string): MediaQueryList | undefined {
  return (window as MaybeMatchMedia).matchMedia?.(query);
}

/** Whether the viewport matches a CSS media query; follows resizing and rotation. */
export function useMediaQuery(query: string): boolean {
  // Stable per query, so React does not resubscribe on every render.
  const subscribe = useCallback(
    (onChange: () => void): (() => void) => {
      const list = mediaQueryList(query);
      list?.addEventListener("change", onChange);
      return () => {
        list?.removeEventListener("change", onChange);
      };
    },
    [query],
  );
  const matches = useCallback(() => mediaQueryList(query)?.matches ?? false, [query]);
  return useSyncExternalStore(subscribe, matches);
}
