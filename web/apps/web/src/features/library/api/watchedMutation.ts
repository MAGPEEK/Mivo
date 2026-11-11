import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import type { QueryClient } from "@tanstack/react-query";

import { itemsQueryKey } from "../../../shared/api";
import { librariesQueryKey } from "./libraryQueries";

const notImplementedStatus = 501;

/**
 * Cache key of "this server stores watch state". It is true until a watch operation answers
 * 501 (servers before M7-23); the controls then explain themselves instead of failing again.
 */
export const watchStateSupportQueryKey = ["watch-state", "supported"] as const;

/** What {@link setItemWatched} sends. */
export interface WatchedRequest {
  readonly itemId: string;
  /** True marks the item watched, false unwatched. */
  readonly watched: boolean;
}

/**
 * Marks an item watched or unwatched for the selected profile; a show or season applies to
 * its episodes. Failures arrive as `ApiError`.
 */
export async function setItemWatched(client: ApiClient, request: WatchedRequest): Promise<void> {
  const params = { path: { itemId: request.itemId } };
  await requestData(
    request.watched
      ? client.PUT("/api/v1/items/{itemId}/watched", { params })
      : client.DELETE("/api/v1/items/{itemId}/watched", { params }),
  );
}

/** Whether a failed watch operation means the server cannot store watch state at all. */
export function isWatchStateUnsupported(error: unknown): boolean {
  return error instanceof ApiError && error.status === notImplementedStatus;
}

/**
 * Reloads everything that shows watch state: the item lists, rows and facets of the
 * libraries (Home rows included) and every loaded item with its children.
 */
export async function invalidateWatchState(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: librariesQueryKey }),
    queryClient.invalidateQueries({ queryKey: itemsQueryKey }),
  ]);
}
