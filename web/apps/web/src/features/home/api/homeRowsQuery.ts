import { requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

import { librariesQueryKey } from "../../library";

/**
 * Cache key of `GET /api/v1/home/rows`. It sits below the libraries key, so whatever refreshes
 * the libraries (a finished scan, a new or removed library) refreshes Home too.
 */
export const homeRowsQueryKey = [...librariesQueryKey, "home-rows"] as const;

// One retry rides out a server restart; more would only delay the error state.
const homeRowsRetries = 1;

/** Query options for the Home rows and hero; each row holds the server's default item count. */
export function homeRowsQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: homeRowsQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/home/rows", { signal })),
    retry: homeRowsRetries,
  });
}
