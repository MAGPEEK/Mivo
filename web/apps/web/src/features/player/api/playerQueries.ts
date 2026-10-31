import { requestData, type ApiClient } from "@mivo/api-client";
import type { ClientProfile } from "@mivo/player-core";
import { queryOptions } from "@tanstack/react-query";

/** Cache key of the client profile; one query client lives for one page load. */
export const clientProfileQueryKey = ["player", "clientProfile"] as const;

/**
 * Query options that detect the client profile once per page load (ADR 0023): the query
 * client lives as long as the page, and the result never goes stale or out of the cache.
 */
export function clientProfileQueryOptions(detect: () => Promise<ClientProfile>) {
  return queryOptions({
    queryKey: clientProfileQueryKey,
    // Called without arguments: the query context must not reach `detect` as its environment.
    queryFn: () => detect(),
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
}

/** Cache key of the item the player opens. */
export function playerItemQueryKey(itemId: string) {
  return ["player", "item", itemId] as const;
}

/**
 * Query options for the item the player opens: it names the primary file and the duration
 * the start position is clamped to. Never refetched in the background, because a changed
 * duration would restart the running session.
 */
export function playerItemQueryOptions(client: ApiClient, itemId: string) {
  return queryOptions({
    queryKey: playerItemQueryKey(itemId),
    queryFn: ({ signal }) =>
      requestData(client.GET("/api/v1/items/{itemId}", { params: { path: { itemId } }, signal })),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
}
