import { useCallback, useState } from "react";

import type { LibraryItems } from "./useLibraryItems";

/** Which card takes focus after "Load more", and the call that requests it. */
export interface NewPageFocus {
  /** ID of the first card of the last page loaded by "Load more"; focus it when it mounts. */
  readonly focusItemId: string | undefined;
  /** Call right before loading the next page. */
  readonly markLoadMore: () => void;
}

interface PendingFocus {
  /** ID of the last card shown at the click; the first new card follows it. */
  readonly afterItemId: string | undefined;
  /** `failureCount` at the click: only a failure after it ends the request. */
  readonly failuresBefore: number;
}

/** The item after `afterItemId`, or undefined while it has not arrived. */
function itemAfter(items: LibraryItems["items"], afterItemId: string | undefined) {
  const index = items.findIndex((item) => item.id === afterItemId);
  return index === -1 ? undefined : items[index + 1];
}

/**
 * Picks the first item of the page that "Load more" requested as soon as it arrives, and then
 * forgets the request. It is found as the item after the last card at the click, not by
 * position, so items a background refresh adds above do not shift the target. Cards are keyed
 * by ID and mount once, so a later refresh cannot move focus again. A failed page drops the
 * request too; a retry marks it again.
 */
export function useNewPageFocus(state: LibraryItems): NewPageFocus {
  const { items, loadMoreFailed, failureCount } = state;
  const [pending, setPending] = useState<PendingFocus>();
  const [focusItemId, setFocusItemId] = useState<string>();
  // Adjusting state while rendering (instead of in an effect) lets the new card mount with
  // its focus flag already set.
  if (pending !== undefined) {
    const firstNewItem = itemAfter(items, pending.afterItemId);
    if (firstNewItem !== undefined) {
      setFocusItemId(firstNewItem.id);
      setPending(undefined);
    } else if (loadMoreFailed && failureCount > pending.failuresBefore) {
      // Compared by count: right after a retry click the old failure is still reported.
      setPending(undefined);
    }
  }
  const lastItemId = items.at(-1)?.id;
  const markLoadMore = useCallback(() => {
    setPending({ afterItemId: lastItemId, failuresBefore: failureCount });
  }, [lastItemId, failureCount]);
  return { focusItemId, markLoadMore };
}
