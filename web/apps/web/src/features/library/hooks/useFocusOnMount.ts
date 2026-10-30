import { useEffect, useRef, type RefObject } from "react";

/**
 * Moves keyboard focus to the returned element once it mounts, when `enabled`. Used for the
 * first card of a page loaded with "Load more": the button may vanish after the last page,
 * and focus must not fall back to the start of the document.
 */
export function useFocusOnMount<Element extends HTMLElement>(
  enabled: boolean,
): RefObject<Element | null> {
  const elementRef = useRef<Element>(null);
  // Runs only on mount on purpose: a card that is already shown never steals focus.
  const enabledOnMount = useRef(enabled);
  useEffect(() => {
    if (enabledOnMount.current) {
      elementRef.current?.focus();
    }
  }, []);
  return elementRef;
}
