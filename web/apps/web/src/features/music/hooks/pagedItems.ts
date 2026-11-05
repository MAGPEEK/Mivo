import type { ItemPage, ItemSummary } from "@mivo/api-client";
import type { InfiniteData, UseInfiniteQueryResult } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

/** Loaded items of a paged listing and the controls to load more. */
export interface PagedItems {
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
  /** Loads the next page; ignored while one is loading. */
  readonly loadMore: () => void;
  /** Reloads after a failed first page. */
  readonly retry: () => void;
}

function pagedStatus(hasPages: boolean, isError: boolean): PagedItems["status"] {
  if (hasPages) {
    return "success";
  }
  return isError ? "error" : "pending";
}

/** The {@link PagedItems} view of an infinite query over item pages. */
export function usePagedItems(query: UseInfiniteQueryResult<InfiniteData<ItemPage>>): PagedItems {
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
    status: pagedStatus(data !== undefined, query.isError),
    items,
    hasMore: query.hasNextPage,
    loadingMore: isFetchingNextPage,
    loadMoreFailed: query.isFetchNextPageError,
    loadMore,
    retry,
  };
}
