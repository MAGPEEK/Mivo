import type { Row } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { isNotImplemented, libraryRowsQueryOptions } from "../api/libraryQueries";
import { overviewHint, type OverviewHint } from "../lib/serverRows";

/** What the Overview pivot shows. */
export type LibraryRowsState =
  | { readonly state: "loading" }
  | {
      readonly state: "error";
      /** The server cannot generate rows yet (501); asking again does not help. */
      readonly notSupported: boolean;
      readonly retry: () => void;
    }
  | {
      readonly state: "loaded";
      /** The rows in server order. */
      readonly rows: readonly Row[];
      /** Shown instead of the rows when there are none, or below them. */
      readonly hint: OverviewHint | undefined;
    };

/**
 * The Overview rows of a library (LB-02...04, LB-09) as the server generated them, with the
 * loading, error and hint states. A failed refresh keeps the rows already shown.
 */
export function useLibraryRows(libraryId: string): LibraryRowsState {
  const client = useApiClient();
  const query = useQuery(libraryRowsQueryOptions(client, libraryId));
  if (query.data === undefined) {
    return query.isError && !query.isFetching
      ? {
          state: "error",
          notSupported: isNotImplemented(query.error),
          retry: () => void query.refetch(),
        }
      : { state: "loading" };
  }
  // The contract requires the list; anything else counts as no rows.
  const rows = Array.isArray(query.data.rows) ? query.data.rows : [];
  return { state: "loaded", rows, hint: overviewHint(query.data.hint, rows.length) };
}
