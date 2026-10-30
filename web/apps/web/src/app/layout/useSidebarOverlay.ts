import { useRouter } from "@tanstack/react-router";
import { useEffect, type RefObject } from "react";

/** Options of {@link useSidebarOverlay}. */
export interface SidebarOverlayOptions {
  /** Whether the sidebar currently lies over the page (narrow screens only). */
  readonly open: boolean;
  readonly close: () => void;
  /** The navigation element; presses inside it keep the overlay open. */
  readonly navigationRef: RefObject<HTMLElement | null>;
  /** The sidebar toggle; Escape returns focus to it. */
  readonly toggleRef: RefObject<HTMLButtonElement | null>;
}

const escapeKey = "Escape";

/**
 * Dismisses the sidebar overlay of narrow screens the way a disclosure navigation menu is
 * dismissed (WAI-ARIA APG): Escape closes it and returns focus to the toggle; a press outside,
 * focus moving outside and every navigation close it. It stays non-modal, so there is no focus
 * trap, and closing on focus leaving means a keyboard user never tabs onto controls hidden
 * under it (WCAG 2.4.11 Focus Not Obscured).
 */
export function useSidebarOverlay(options: SidebarOverlayOptions): void {
  const { open, close, navigationRef, toggleRef } = options;
  const router = useRouter();

  // Every navigation closes it, including links inside the overlay: the new page must not
  // start covered.
  useEffect(() => router.subscribe("onResolved", close), [router, close]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== escapeKey || event.defaultPrevented) {
        return;
      }
      close();
      toggleRef.current?.focus();
    };
    const closeOnPressOutside = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && navigationRef.current?.contains(target) !== true) {
        close();
      }
    };
    // focusout bubbles, so one listener sees every control in the navigation. relatedTarget
    // is null when the window loses focus: switching apps does not count as leaving.
    const closeOnFocusLeave = (event: FocusEvent) => {
      const next = event.relatedTarget;
      if (next instanceof Node && navigationRef.current?.contains(next) !== true) {
        close();
      }
    };
    const navigation = navigationRef.current;
    document.addEventListener("keydown", closeOnEscape);
    document.addEventListener("pointerdown", closeOnPressOutside);
    navigation?.addEventListener("focusout", closeOnFocusLeave);
    return () => {
      document.removeEventListener("keydown", closeOnEscape);
      document.removeEventListener("pointerdown", closeOnPressOutside);
      navigation?.removeEventListener("focusout", closeOnFocusLeave);
    };
  }, [open, close, navigationRef, toggleRef]);
}
