import type { ItemSummary } from "@mivo/api-client";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import { libraryItemsQueryOptions } from "../api/libraryQueries";

/** The loaded items of a library and the controls to load more. */
export interface LibraryItems {
  /**
   * `error` only when nothing could be loaded. Once a page is shown it stays `success`: a
   * failed next page is {@link loadMoreFailed}, a failed background refresh keeps the pages.
   */
  readonly status: "pending" | "error" | "success";
  /** Every item of the pages loaded so far, in server order. */
  readonly items: readonly ItemSummary[];
  readonly hasMore: boolean;
  readonly loadingMore: boolean;
  /** The last "Load more" failed; calling {@link loadMore} again retries it. */
  readonly loadMoreFailed: boolean;
  /** Grows with every failed request, so a new failure can be told from an old one. */
  readonly failureCount: number;
  /** Loads the next page; ignored while one is loading. */
  readonly loadMore: () => void;
  /** Reloads after a failed first page. */
  readonly retry: () => void;
}

function itemsStatus(hasPages: boolean, isError: boolean): LibraryItems["status"] {
  if (hasPages) {
    return "success";
  }
  return isError ? "error" : "pending";
}

/** Pages through the items of a library with the server's `nextCursor`. */
export function useLibraryItems(libraryId: string): LibraryItems {
  const client = useApiClient();
  const query = useInfiniteQuery(libraryItemsQueryOptions(client, libraryId));
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
    status: itemsStatus(data !== undefined, query.isError),
    items,
    hasMore: query.hasNextPage,
    loadingMore: isFetchingNextPage,
    loadMoreFailed: query.isFetchNextPageError,
    failureCount: query.errorUpdateCount,
    loadMore,
    retry,
  };
}
