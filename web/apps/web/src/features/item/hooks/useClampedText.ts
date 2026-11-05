import { useCallback, useLayoutEffect, useRef, useState, type RefObject } from "react";

/** State of a text clamped to a few lines with a "More"/"Less" toggle. */
export interface ClampedText {
  /** Goes on the clamped element, so the hook can see whether the text overflows. */
  readonly ref: RefObject<HTMLParagraphElement | null>;
  readonly expanded: boolean;
  /** Whether to offer the toggle: the text is cut off, or it is open and can close again. */
  readonly showToggle: boolean;
  readonly toggle: () => void;
}

/**
 * Clamps `text` and offers a toggle only when the clamp actually cuts it off, measured after
 * layout and again whenever the element is resized. A new text starts clamped again.
 */
export function useClampedText(text: string): ClampedText {
  const ref = useRef<HTMLParagraphElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [overflows, setOverflows] = useState(false);
  const [shownText, setShownText] = useState(text);
  // Adjusting state while rendering resets the toggle before the new text paints.
  if (shownText !== text) {
    setShownText(text);
    setExpanded(false);
  }
  useLayoutEffect(() => {
    const element = ref.current;
    if (element === null || expanded) {
      return undefined;
    }
    const measure = () => {
      setOverflows(element.scrollHeight > element.clientHeight);
    };
    measure();
    // jsdom and old browsers have no ResizeObserver; the first measurement then has to do.
    if (typeof ResizeObserver !== "function") {
      return undefined;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => {
      observer.disconnect();
    };
  }, [text, expanded]);
  const toggle = useCallback(() => {
    setExpanded((open) => !open);
  }, []);
  return { ref, expanded, showToggle: expanded || overflows, toggle };
}
