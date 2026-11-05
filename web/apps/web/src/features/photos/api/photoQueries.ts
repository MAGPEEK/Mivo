import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import { infiniteQueryOptions } from "@tanstack/react-query";

import { itemQueryKey } from "../../../shared/api";

/**
 * Sub-albums and photos per page. Without grid virtualisation (milestone 5) a page adds this
 * many tiles to the DOM, so the API cursor keeps it bounded.
 */
export const albumContentsPageSize = 120;

// One retry rides out a server restart; the server rejecting the request itself (4xx: unknown
// item, invalid cursor) gets the same answer again.
const photoRetries = 1;
const clientErrorStatusMin = 400;
const serverErrorStatusMin = 500;

/** Retry policy of the photo queries: network and server errors are retried, once. */
export function shouldRetryPhotoQuery(failureCount: number, error: Error): boolean {
  const isClientError =
    error instanceof ApiError &&
    error.status >= clientErrorStatusMin &&
    error.status < serverErrorStatusMin;
  return !isClientError && failureCount < photoRetries;
}

/**
 * Cache key of the content pages of a photo album: below the album's item key, so invalidating
 * the album reaches its contents.
 */
export function albumContentsQueryKey(albumId: string) {
  return [...itemQueryKey(albumId), "contents"] as const;
}

/**
 * Infinite query options for the contents of a photo album: its sub-albums first, then its
 * photos by capture time. The server lists albums in this order only, so no `order` is sent
 * (`order=desc` is rejected).
 */
export function albumContentsQueryOptions(client: ApiClient, albumId: string) {
  return infiniteQueryOptions({
    queryKey: albumContentsQueryKey(albumId),
    queryFn: ({ pageParam, signal }) =>
      requestData(
        client.GET("/api/v1/items/{itemId}/children", {
          params: {
            path: { itemId: albumId },
            query: {
              limit: albumContentsPageSize,
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
            },
          },
          signal,
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: shouldRetryPhotoQuery,
  });
}
