import type { LibraryFacetsQuery } from "@mivo/api-client";
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { isNotImplemented, libraryFacetsQueryOptions } from "../api/libraryQueries";
import { facetsByField, type Facets, type FacetsByField } from "../lib/facetOptions";

const noFacets: FacetsByField = new Map();

function facetsStatus(hasData: boolean, isError: boolean): Facets["status"] {
  if (hasData) {
    return "success";
  }
  return isError ? "error" : "pending";
}

/**
 * Loads the facet counts of a library under `facetQuery` once `enabled`. A new query (another
 * filter) loads again while the old counts stay visible; answers to an older query land in
 * their own cache entry, so they never replace the current counts.
 */
export function useFacets(
  libraryId: string,
  facetQuery: LibraryFacetsQuery,
  enabled: boolean,
): Facets {
  const client = useApiClient();
  const query = useQuery({
    ...libraryFacetsQueryOptions(client, libraryId, facetQuery),
    enabled,
    placeholderData: keepPreviousData,
    select: facetsByField,
  });
  return {
    status: facetsStatus(query.data !== undefined, query.isError),
    notSupported: isNotImplemented(query.error),
    byField: query.data ?? noFacets,
  };
}
