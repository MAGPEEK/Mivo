import { useEffect, useRef, type RefObject } from "react";

/**
 * Returns a ref whose element takes the focus whenever `value` changes after the first render.
 * For controls that replace each other in place ("Change PIN" becomes the PIN field): without
 * it the focus would fall back to the page when the focused button disappears.
 */
export function useFocusOnChange<Element extends HTMLElement>(
  value: unknown,
): RefObject<Element | null> {
  const ref = useRef<Element>(null);
  const previous = useRef(value);
  useEffect(() => {
    if (previous.current !== value) {
      previous.current = value;
      ref.current?.focus();
    }
  }, [value]);
  return ref;
}
