import { useInfiniteQuery } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { childrenQueryOptions } from "../api/childrenQuery";
import { toChildRow, type ChildRow, type ChildRowTexts } from "../lib/childRow";
import type { DurationParts } from "../lib/duration";
import type { ChildOrder } from "../lib/itemSearch";
import { itemNamespace } from "../locales";
import { durationText } from "./durationText";

type ItemTranslate = TFunction<typeof itemNamespace>;

/** The loaded children of a show or season and the controls to load more. */
export interface ItemChildren {
  /**
   * `error` only when nothing could be loaded. Once a page is shown it stays `success`: a
   * failed next page is {@link loadMoreFailed}, a failed background refresh keeps the pages.
   */
  readonly status: "pending" | "error" | "success";
  readonly rows: readonly ChildRow[];
  readonly hasMore: boolean;
  readonly loadingMore: boolean;
  /** The last "Load more" failed; calling {@link loadMore} again retries it. */
  readonly loadMoreFailed: boolean;
  readonly loadMore: () => void;
  /** Reloads after a failed first page. */
  readonly retry: () => void;
}

function rowTexts(t: ItemTranslate): ChildRowTexts {
  return {
    seasonName: (number) => t("season.name", { number }),
    specials: t("season.specials"),
    seasonCaption: (name, episodeCount) =>
      t("season.caption", { name, episodes: t("episodes", { count: episodeCount }) }),
    episodeNumber: ({ first, last }) =>
      first === last
        ? t("episode.number", { number: first })
        : t("episode.numbers", { first, last }),
    duration: (parts: DurationParts) => durationText(parts, t),
  };
}

function childrenStatus(hasPages: boolean, isError: boolean): ItemChildren["status"] {
  if (hasPages) {
    return "success";
  }
  return isError ? "error" : "pending";
}

/** Pages through the children (seasons or episodes) of an item with the server's cursor. */
export function useItemChildren(itemId: string, order: ChildOrder): ItemChildren {
  const client = useApiClient();
  const { t } = useTranslation(itemNamespace);
  const query = useInfiniteQuery(childrenQueryOptions(client, itemId, order));
  const { data, fetchNextPage, refetch, isFetchingNextPage } = query;
  const rows = useMemo(
    () =>
      data?.pages.flatMap((page) => page.items).map((item) => toChildRow(item, rowTexts(t))) ?? [],
    [data, t],
  );
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
    status: childrenStatus(data !== undefined, query.isError),
    rows,
    hasMore: query.hasNextPage,
    loadingMore: isFetchingNextPage,
    loadMoreFailed: query.isFetchNextPageError,
    loadMore,
    retry,
  };
}
