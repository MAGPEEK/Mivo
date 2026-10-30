import { requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Cache key of `GET /api/v1/system/info`. */
export const systemInfoQueryKey = ["system", "info"] as const;

// A failed load is retried once: enough to ride out a server restart, short enough to show
// "Server unreachable" soon when it is really down.
const systemInfoRetries = 1;

/**
 * Query options for the server's system info. The version and identity only change when the
 * server restarts, which reloads the app anyway, so the data never goes stale.
 */
export function systemInfoQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: systemInfoQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/system/info", { signal })),
    staleTime: Infinity,
    retry: systemInfoRetries,
  });
}
