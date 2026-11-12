import { useMutation, useQueryClient } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import {
  hideFromContinueWatching,
  invalidateWatchState,
  setItemWatched,
} from "../api/watchedMutation";

/** The overflow actions of a Continue watching card (HO-03, LB-01). */
export interface ContinueWatchingActions {
  /** Hides the item from the rows; its resume position stays. */
  readonly remove: () => void;
  /** Marks the item watched, which also clears its resume position. */
  readonly markWatched: () => void;
  /** True while a request or the reload of the rows after it runs. */
  readonly pending: boolean;
  /** True when the last action failed. */
  readonly failed: boolean;
}

type Action = "remove" | "markWatched";

/**
 * Remove and "Mark as watched" of one Continue watching card. After a success the rows, lists
 * and items are reloaded, so the card disappears from the row; the request stays pending until
 * they have arrived.
 */
export function useContinueWatchingActions(itemId: string): ContinueWatchingActions {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: (action: Action) =>
      action === "remove"
        ? hideFromContinueWatching(client, itemId)
        : setItemWatched(client, { itemId, watched: true }),
    onSuccess: () => invalidateWatchState(queryClient),
  });
  const run = (action: Action) => () => {
    if (!mutation.isPending) {
      mutation.mutate(action);
    }
  };
  return {
    remove: run("remove"),
    markWatched: run("markWatched"),
    pending: mutation.isPending,
    failed: mutation.isError,
  };
}
