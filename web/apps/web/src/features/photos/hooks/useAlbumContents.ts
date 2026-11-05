import { useInfiniteQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import type { LibraryItems } from "../../library";
import { albumContentsQueryOptions } from "../api/photoQueries";

/**
 * Pages through the sub-albums and photos of a photo album with the server's cursor. The result
 * has the shape of a library listing, so the album page reuses the library's Browse grid with
 * its states, "Load more" and focus handling.
 */
export function useAlbumContents(albumId: string): LibraryItems {
  const client = useApiClient();
  const query = useInfiniteQuery(albumContentsQueryOptions(client, albumId));
  const { data, fetchNextPage, refetch, isFetchingNextPage } = query;
  const items = useMemo(() => data?.pages.flatMap((page) => page.items) ?? [], [data]);
  const loadMore = useCallback(() => {
    // fetchNextPage would cancel and restart a running request; a double click must not.
    if (!isFetchingNextPage) {
      void fetchNextPage();
    }
  }, [fetchNextPage, isFetchingNextPage]);
  const retry = useCallback(() => {
    void refetch();
  }, [refetch]);
  return {
    status: contentsStatus(data !== undefined, query.isError),
    // The children endpoint takes no filters, so the server never answers "not implemented".
    notSupported: false,
    items,
    totalCount: data?.pages[0]?.totalCount,
    hasMore: query.hasNextPage,
    loadingMore: isFetchingNextPage,
    loadMoreFailed: query.isFetchNextPageError,
    failureCount: query.errorUpdateCount,
    loadMore,
    retry,
  };
}

function contentsStatus(hasPages: boolean, isError: boolean): LibraryItems["status"] {
  if (hasPages) {
    return "success";
  }
  return isError ? "error" : "pending";
}
