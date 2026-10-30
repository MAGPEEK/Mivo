import type { Library } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { isClientError, libraryQueryOptions } from "../api/libraryQueries";

/** What the library page shows. */
export type LibraryState =
  | { readonly status: "pending" }
  | { readonly status: "error"; readonly error: Error; readonly retry: () => void }
  | { readonly status: "success"; readonly library: Library };

/**
 * Loads one library with its scan status and keeps polling it while a scan runs. A poll that
 * fails with a network or server error keeps showing the last known library. A client error
 * (4xx, e.g. 404 after the library was removed from the configuration) does not: the same
 * request will keep failing, and polling stops for it as well. Failures arrive as `ApiError`.
 */
export function useLibrary(libraryId: string): LibraryState {
  const client = useApiClient();
  const query = useQuery(libraryQueryOptions(client, libraryId));
  const { data, error, refetch } = query;
  if (data !== undefined && !isClientError(error)) {
    return { status: "success", library: data };
  }
  if (error !== null) {
    return {
      status: "error",
      error,
      retry: () => {
        void refetch();
      },
    };
  }
  return { status: "pending" };
}
