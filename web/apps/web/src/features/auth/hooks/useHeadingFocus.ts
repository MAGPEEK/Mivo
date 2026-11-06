import { useEffect, useRef, type RefObject } from "react";

/**
 * Focuses the heading once when the page opens, so keyboard and screen-reader users start at
 * the top of a page the app switched to on its own (sign-in after a lost session, setup).
 */
export function useHeadingFocus(): RefObject<HTMLHeadingElement | null> {
  const headingRef = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    headingRef.current?.focus();
  }, []);
  return headingRef;
}
