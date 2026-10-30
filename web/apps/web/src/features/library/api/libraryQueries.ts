import {
  ApiError,
  requestData,
  type ApiClient,
  type Library,
  type ScanStatus,
} from "@mivo/api-client";
import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";

/** Prefix of every library cache key; invalidating it refreshes lists, details and items. */
export const librariesQueryKey = ["libraries"] as const;

/** Cache key of `GET /api/v1/libraries`. */
export const libraryListQueryKey = [...librariesQueryKey, "list"] as const;

/** Cache key of `GET /api/v1/libraries/{libraryId}`. */
export function libraryQueryKey(libraryId: string) {
  return [...librariesQueryKey, "detail", libraryId] as const;
}

/** Cache key of the pages of `GET /api/v1/libraries/{libraryId}/items`. */
export function libraryItemsQueryKey(libraryId: string) {
  return [...libraryQueryKey(libraryId), "items"] as const;
}

/** How often the library is reloaded while its scan runs (OD-3 A, ADR 0022). */
export const scanStatusPollIntervalMs = 2_000;

/**
 * Items per page. Without virtualisation (milestone 5) a page adds this many cards to the
 * DOM, so it stays well below the server maximum of 500.
 */
export const libraryItemsPageSize = 60;

// One retry rides out a server restart; more would only delay the error state.
const libraryRetries = 1;
const clientErrorStatusMin = 400;
const serverErrorStatusMin = 500;

/**
 * Whether the server rejected the request itself (4xx: unknown library, invalid cursor). The
 * same request gets the same answer, so retrying or polling it is pointless.
 */
export function isClientError(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status >= clientErrorStatusMin &&
    error.status < serverErrorStatusMin
  );
}

/** Retry policy of every library query: network and server errors are retried, once. */
export function shouldRetryLibraryQuery(failureCount: number, error: Error): boolean {
  return !isClientError(error) && failureCount < libraryRetries;
}

/** Whether `scan` is still running. */
export function isScanRunning(scan: ScanStatus | undefined): boolean {
  return scan?.state === "running";
}

/** Query options for every configured library, ordered by name. */
export function libraryListQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: libraryListQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/libraries", { signal })),
    // The contract requires the list; a server or proxy answering with another body must not
    // take the whole layout down with it, so anything but an array counts as empty.
    select: (list) => (Array.isArray(list.libraries) ? list.libraries : []),
    retry: shouldRetryLibraryQuery,
  });
}

/**
 * Query options for one library. While its scan runs, the library is reloaded every
 * {@link scanStatusPollIntervalMs}, so progress and the result appear without a reload.
 */
export function libraryQueryOptions(client: ApiClient, libraryId: string) {
  return queryOptions({
    queryKey: libraryQueryKey(libraryId),
    queryFn: ({ signal }) =>
      requestData(
        client.GET("/api/v1/libraries/{libraryId}", { params: { path: { libraryId } }, signal }),
      ),
    retry: shouldRetryLibraryQuery,
    // A failed poll keeps the last data, which still says "running"; a client error (e.g. the
    // library was removed from the configuration) will not go away, so polling stops.
    refetchInterval: (query) =>
      isScanRunning(query.state.data?.scan) && !isClientError(query.state.error)
        ? scanStatusPollIntervalMs
        : false,
  });
}

/** Infinite query options that page through a library's items with `nextCursor`. */
export function libraryItemsQueryOptions(client: ApiClient, libraryId: string) {
  return infiniteQueryOptions({
    queryKey: libraryItemsQueryKey(libraryId),
    queryFn: ({ pageParam, signal }) =>
      requestData(
        client.GET("/api/v1/libraries/{libraryId}/items", {
          params: {
            path: { libraryId },
            query: {
              limit: libraryItemsPageSize,
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
            },
          },
          signal,
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: shouldRetryLibraryQuery,
  });
}

/** Starts a scan of the library; resolves with the status of the running scan. */
export function startLibraryScan(client: ApiClient, libraryId: string): Promise<ScanStatus> {
  return requestData(
    client.POST("/api/v1/libraries/{libraryId}/scan", { params: { path: { libraryId } } }),
  );
}

/** `library` with `scan` as its scan status; keeps an absent library absent. */
export function withScanStatus(
  library: Library | undefined,
  scan: ScanStatus,
): Library | undefined {
  return library === undefined ? undefined : { ...library, scan };
}
