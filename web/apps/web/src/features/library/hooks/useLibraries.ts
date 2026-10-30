import type { Library } from "@mivo/api-client";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { libraryListQueryOptions } from "../api/libraryQueries";

/** Loads every configured library, ordered by name. Failures arrive as `ApiError`. */
export function useLibraries(): UseQueryResult<Library[]> {
  const client = useApiClient();
  return useQuery(libraryListQueryOptions(client));
}
