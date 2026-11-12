import {
  ApiError,
  type ItemSummary,
  type LibraryFolderEntry,
  type LibraryFolderListing,
} from "@mivo/api-client";
import {
  useInfiniteQuery,
  type InfiniteData,
  type UseInfiniteQueryResult,
} from "@tanstack/react-query";
import { useCallback, useEffect, useMemo } from "react";

import { useApiClient } from "../../../shared/api";
import { libraryFoldersQueryOptions } from "../api/libraryQueries";
import type { LibraryItems } from "./useLibraryItems";

const notFoundStatus = 404;
const badRequestStatus = 400;

/** What the folder view shows for one folder of a library. */
export type FolderListing =
  | { readonly status: "pending" }
  /** The server has no such folder (404), or rejected the path (400). */
  | { readonly status: "notFound" }
  | { readonly status: "error"; readonly retry: () => void }
  | {
      readonly status: "success";
      /** Sub-folders of every page loaded so far, in server order (they come first). */
      readonly folders: readonly LibraryFolderEntry[];
      /** The items of the loaded pages as the virtual grid's data source. */
      readonly items: LibraryItems;
    };

function isMissingFolder(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    (error.status === notFoundStatus || error.status === badRequestStatus)
  );
}

function splitEntries(entries: readonly LibraryFolderEntry[]) {
  const folders: LibraryFolderEntry[] = [];
  const items: ItemSummary[] = [];
  for (const entry of entries) {
    if (entry.kind === "folder") {
      folders.push(entry);
    } else if (entry.item !== undefined) {
      items.push(entry.item);
    }
    // Entry kinds a later server version adds are skipped (the contract asks clients to cope).
  }
  return { folders, items };
}

type FolderQuery = UseInfiniteQueryResult<InfiniteData<LibraryFolderListing>>;

// The loaded items shaped like a library listing, with the paging controls of the query.
function useFolderItems(query: FolderQuery, items: readonly ItemSummary[]): LibraryItems {
  const { fetchNextPage, refetch, isFetchingNextPage, hasNextPage, isFetchNextPageError } = query;
  const loadMore = useCallback(() => {
    // fetchNextPage would cancel and restart a running request; a double click must not.
    if (!isFetchingNextPage) {
      void fetchNextPage();
    }
  }, [fetchNextPage, isFetchingNextPage]);
  const retry = useCallback(() => {
    void refetch();
  }, [refetch]);
  const failureCount = query.errorUpdateCount;
  // Folders come first, so a page may hold none of the items; the grid is not there to ask for
  // more, so keep loading until an item shows or the folder is complete.
  const waitingForItems = query.data !== undefined && items.length === 0;
  useEffect(() => {
    if (waitingForItems && hasNextPage && !isFetchNextPageError) {
      loadMore();
    }
  }, [waitingForItems, hasNextPage, isFetchNextPageError, loadMore]);
  return useMemo<LibraryItems>(
    () => ({
      status: "success",
      notSupported: false,
      items,
      // The listing counts folders too, so the item count is known only once every page is in.
      totalCount: hasNextPage ? undefined : items.length,
      hasMore: hasNextPage,
      loadingMore: isFetchingNextPage,
      loadMoreFailed: isFetchNextPageError,
      failureCount,
      loadMore,
      retry,
    }),
    [items, hasNextPage, isFetchingNextPage, isFetchNextPageError, failureCount, loadMore, retry],
  );
}

/**
 * Pages through the children of the library folder `path` (library-relative, validated by the
 * caller). The items are shaped like a library listing, so the virtual grid and its views show
 * them unchanged.
 */
export function useFolderListing(libraryId: string, path: string): FolderListing {
  const client = useApiClient();
  const query = useInfiniteQuery(libraryFoldersQueryOptions(client, libraryId, path));
  const { data, refetch } = query;
  const split = useMemo(
    () => splitEntries(data?.pages.flatMap((page) => page.entries) ?? []),
    [data],
  );
  const items = useFolderItems(query, split.items);
  if (data !== undefined) {
    return { status: "success", folders: split.folders, items };
  }
  if (!query.isError) {
    return { status: "pending" };
  }
  if (isMissingFolder(query.error)) {
    return { status: "notFound" };
  }
  return {
    status: "error",
    retry: () => {
      void refetch();
    },
  };
}
