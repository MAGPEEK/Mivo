import { requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Cache key of `GET /api/v1/system/info`. */
export const systemInfoQueryKey = ["system", "info"] as const;

// A failed load is retried once: enough to ride out a server restart, short enough to show
// "Server unreachable" soon when it is really down.
const systemInfoRetries = 1;

/**
 * Query options for the server's system info. The version and identity only change when the
 * server restarts or is updated. The app does not reload itself then, so it keeps showing the
 * previous version until the page is reloaded; that is acceptable for a footer label and
 * saves a request on every mount.
 */
export function systemInfoQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: systemInfoQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/system/info", { signal })),
    staleTime: Infinity,
    retry: systemInfoRetries,
  });
}
