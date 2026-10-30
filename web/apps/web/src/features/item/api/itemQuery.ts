import { ApiError, clientErrorCodes, requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

const badRequestStatus = 400;
const notFoundStatus = 404;

// One retry rides out a server restart without keeping the user waiting long for the retry
// button when the server is really down.
const itemRetries = 1;

/** Cache key of `GET /api/v1/items/{itemId}`. */
export function itemQueryKey(itemId: string) {
  return ["items", itemId] as const;
}

/**
 * Whether a failed item load means "there is no such item". Besides 404 that includes 400: the
 * only input is the ID from the URL, so the server rejects it only when it is no item ID at
 * all, e.g. a hand-edited link. Retrying either cannot succeed.
 */
export function isItemNotFound(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    (error.status === notFoundStatus || error.status === badRequestStatus)
  );
}

/** Whether a failed item load never reached the server (offline, refused, DNS, ...). */
export function isNetworkFailure(error: unknown): boolean {
  return error instanceof ApiError && error.code === clientErrorCodes.networkError;
}

/** Query options for one item with its media files and streams. */
export function itemQueryOptions(client: ApiClient, itemId: string) {
  return queryOptions({
    queryKey: itemQueryKey(itemId),
    queryFn: ({ signal }) =>
      requestData(client.GET("/api/v1/items/{itemId}", { params: { path: { itemId } }, signal })),
    retry: (failureCount, error) => !isItemNotFound(error) && failureCount < itemRetries,
  });
}
