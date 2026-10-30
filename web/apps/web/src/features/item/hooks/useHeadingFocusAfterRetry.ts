import { useEffect, useRef, type RefObject } from "react";

import type { ItemDetailsState } from "./useItemDetails";

/** What {@link useHeadingFocusAfterRetry} hands to the page. */
export interface HeadingFocusAfterRetry {
  /** Attach to the page's h1 (with `tabIndex={-1}`). */
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
  /** Wraps a retry so that focus moves to the heading once the retry settles. */
  readonly focusAfter: (retry: () => void) => () => void;
}

/**
 * Moves keyboard focus to the page heading when a retry settles. The "Try again" button
 * disappears while the item loads, which would drop focus to the document; the heading of
 * the loaded item or of the repeated error tells screen reader users how it ended.
 */
export function useHeadingFocusAfterRetry(state: ItemDetailsState): HeadingFocusAfterRetry {
  const headingRef = useRef<HTMLHeadingElement>(null);
  const focusPending = useRef(false);
  // Keyed on the settlement, not on the status: a quick success can go from "error" to
  // "ready" without a loading render in between, and a repeated error keeps the status.
  const settlement = "settlement" in state ? state.settlement : undefined;
  useEffect(() => {
    if (settlement !== undefined && focusPending.current) {
      focusPending.current = false;
      headingRef.current?.focus();
    }
  }, [settlement]);
  const focusAfter = (retry: () => void) => () => {
    focusPending.current = true;
    retry();
  };
  return { headingRef, focusAfter };
}
