import type { ItemDetail, ViewerState } from "@mivo/api-client";
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";

import { itemQueryKey, useApiClient } from "../../../shared/api";
import {
  canMarkWatched,
  expectedViewerState,
  isWatchContainer,
} from "../../../shared/lib/watchState";
import {
  invalidateWatchState,
  isWatchStateUnsupported,
  setItemWatched,
  watchStateSupportQueryKey,
} from "../api/watchedMutation";

/** The part of an item the toggle needs; summaries and details both fit. */
export interface WatchedToggleItem {
  readonly id: string;
  readonly kind: string;
  readonly viewerState?: ViewerState | undefined;
}

/**
 * Why the toggle did not do what was asked: `unsupported` when the server cannot store watch
 * state (it answered 501; the toggle stays off), `failed` when the last request failed.
 */
export type WatchedToggleFailure = "none" | "unsupported" | "failed";

/** "Mark as watched / unwatched" of one item. */
export interface WatchedToggle {
  /** False for kinds without the action (extras, music, photos): show no control. */
  readonly available: boolean;
  /** The state to show: the expected one while a request runs, else the server's. */
  readonly watched: boolean;
  /** Like `watched`, for badges and the resume position; `undefined` without any state. */
  readonly viewerState: ViewerState | undefined;
  readonly pending: boolean;
  /**
   * True when unmarking needs the user's confirmation first: it applies to every episode of a
   * show or season and clears all their resume positions for good.
   */
  readonly confirmUnmark: boolean;
  readonly failure: WatchedToggleFailure;
  /** Marks the item the other way; ignored while a request runs or when unsupported. */
  readonly toggle: () => void;
}

/** The item detail as it was before the expected state was written into the cache. */
interface Rollback {
  readonly previous: ItemDetail | undefined;
}

// Shows the expected state on the item's detail page at once; cards show it from the pending
// request instead, because the list caches have many shapes.
async function writeExpectedState(
  queryClient: QueryClient,
  itemId: string,
  watched: boolean,
): Promise<Rollback> {
  const queryKey = itemQueryKey(itemId);
  // A load still in flight would overwrite the expected state with the one before the click.
  await queryClient.cancelQueries({ queryKey, exact: true });
  const previous = queryClient.getQueryData<ItemDetail>(queryKey);
  if (previous !== undefined) {
    queryClient.setQueryData<ItemDetail>(queryKey, {
      ...previous,
      viewerState: expectedViewerState(previous.viewerState, watched),
    });
  }
  return { previous };
}

// True until a watch operation was answered with 501; shared by every toggle on the page.
function useWatchStateSupported(): boolean {
  const { data } = useQuery({
    queryKey: watchStateSupportQueryKey,
    queryFn: () => true,
    initialData: true,
    staleTime: Infinity,
  });
  return data;
}

/**
 * Marks `item` watched or unwatched. The new state shows at once and is taken back when the
 * request fails; after a success the lists, rows and items are reloaded and the expected state
 * stays until they have arrived.
 */
export function useWatchedToggle(item: WatchedToggleItem): WatchedToggle {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const supported = useWatchStateSupported();
  const itemId = item.id;
  const mutation = useMutation({
    mutationFn: (watched: boolean) => setItemWatched(client, { itemId, watched }),
    onMutate: (watched): Promise<Rollback> => writeExpectedState(queryClient, itemId, watched),
    onError: (error, _watched, rollback) => {
      if (rollback?.previous !== undefined) {
        queryClient.setQueryData(itemQueryKey(itemId), rollback.previous);
      }
      if (isWatchStateUnsupported(error)) {
        queryClient.setQueryData(watchStateSupportQueryKey, false);
      }
    },
    // Awaited, so the request stays pending (and the expected state shown) until reloaded.
    onSuccess: () => invalidateWatchState(queryClient),
  });
  const viewerState = mutation.isPending
    ? expectedViewerState(item.viewerState, mutation.variables)
    : item.viewerState;
  const watched = viewerState?.watched ?? false;
  return {
    available: canMarkWatched(item.kind),
    watched,
    viewerState,
    pending: mutation.isPending,
    confirmUnmark: watched && isWatchContainer(item.kind),
    failure: failureOf(supported, mutation.isError),
    toggle: () => {
      if (supported && !mutation.isPending) {
        mutation.mutate(!watched);
      }
    },
  };
}

function failureOf(supported: boolean, failed: boolean): WatchedToggleFailure {
  if (!supported) {
    return "unsupported";
  }
  return failed ? "failed" : "none";
}
