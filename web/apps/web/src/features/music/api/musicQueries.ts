import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";

import { itemQueryKey } from "../../../shared/api";

/** Tracks per page: the server maximum, so one page holds every track of a usual album. */
export const tracksPageSize = 500;

// One retry rides out a server restart; the server rejecting the request itself (4xx: unknown
// item, invalid cursor) gets the same answer again.
const musicRetries = 1;
const clientErrorStatusMin = 400;
const serverErrorStatusMin = 500;

/** Retry policy of the music queries: network and server errors are retried, once. */
export function shouldRetryMusicQuery(failureCount: number, error: Error): boolean {
  const isClientError =
    error instanceof ApiError &&
    error.status >= clientErrorStatusMin &&
    error.status < serverErrorStatusMin;
  return !isClientError && failureCount < musicRetries;
}

/**
 * Cache key of the track pages of an album: below the album's item key, so invalidating the
 * album reaches its tracks.
 */
export function albumTracksQueryKey(albumId: string) {
  return [...itemQueryKey(albumId), "tracks"] as const;
}

/** Infinite query options for the tracks of an album, in disc and track order. */
export function albumTracksQueryOptions(client: ApiClient, albumId: string) {
  return infiniteQueryOptions({
    queryKey: albumTracksQueryKey(albumId),
    queryFn: ({ pageParam, signal }) =>
      requestData(
        client.GET("/api/v1/items/{itemId}/children", {
          params: {
            path: { itemId: albumId },
            query: {
              limit: tracksPageSize,
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
            },
          },
          signal,
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: shouldRetryMusicQuery,
  });
}

/**
 * Query options for an artist's name. Album summaries name only their artist's ID; the
 * request is shared by every card of the same artist.
 */
export function artistNameQueryOptions(client: ApiClient, artistId: string) {
  return queryOptions({
    queryKey: itemQueryKey(artistId),
    queryFn: ({ signal }) =>
      requestData(
        client.GET("/api/v1/items/{itemId}", { params: { path: { itemId: artistId } }, signal }),
      ),
    select: (artist) => artist.title,
    retry: shouldRetryMusicQuery,
  });
}
