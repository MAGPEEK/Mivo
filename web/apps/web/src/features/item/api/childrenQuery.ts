import { requestData, type ApiClient } from "@mivo/api-client";
import { infiniteQueryOptions } from "@tanstack/react-query";

import type { ChildOrder } from "../lib/itemSearch";
import { isItemNotFound, itemQueryKey } from "./itemQuery";

/**
 * Children per page. Without virtualisation (milestone 5) a page adds this many rows to the
 * DOM; a 131-episode season needs three pages.
 */
export const childrenPageSize = 60;

const childrenRetries = 1;

/** Cache key of the pages of `GET /api/v1/items/{itemId}/children`. */
export function childrenQueryKey(itemId: string, order: ChildOrder) {
  return [...itemQueryKey(itemId), "children", order] as const;
}

/** Infinite query options that page through the children of a show or season. */
export function childrenQueryOptions(client: ApiClient, itemId: string, order: ChildOrder) {
  return infiniteQueryOptions({
    queryKey: childrenQueryKey(itemId, order),
    queryFn: ({ pageParam, signal }) =>
      requestData(
        client.GET("/api/v1/items/{itemId}/children", {
          params: {
            path: { itemId },
            query: {
              limit: childrenPageSize,
              order,
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
            },
          },
          signal,
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: (failureCount, error) => !isItemNotFound(error) && failureCount < childrenRetries,
  });
}
