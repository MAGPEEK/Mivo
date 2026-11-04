import { useEffect, useRef, type RefObject } from "react";

/**
 * Moves keyboard focus to the page heading when the page shows another item than before, e.g.
 * after the previous/next arrows or a link to the show. The first item of a visit keeps the
 * focus where it is. The heading exists once the new item has loaded, so focus waits for that.
 */
export function useHeadingFocusAfterNavigation(
  itemId: string,
  ready: boolean,
  headingRef: RefObject<HTMLHeadingElement | null>,
): void {
  const shownItemId = useRef(itemId);
  const focusPending = useRef(false);
  useEffect(() => {
    if (shownItemId.current !== itemId) {
      shownItemId.current = itemId;
      focusPending.current = true;
    }
    if (ready && focusPending.current) {
      focusPending.current = false;
      headingRef.current?.focus();
    }
  }, [itemId, ready, headingRef]);
}
