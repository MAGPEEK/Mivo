import type { SystemInfo } from "@mivo/api-client";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { systemInfoQueryOptions } from "../api/systemInfoQuery";

/**
 * Loads product name, version and identity of the connected server. Failures arrive as
 * `ApiError` from `@mivo/api-client`.
 */
export function useSystemInfo(): UseQueryResult<SystemInfo> {
  const client = useApiClient();
  return useQuery(systemInfoQueryOptions(client));
}
