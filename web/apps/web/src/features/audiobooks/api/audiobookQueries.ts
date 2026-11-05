import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

import { itemQueryKey } from "../../../shared/api";
import { creditedNames } from "../lib/bookCredits";

// One retry rides out a server restart; the server rejecting the request itself (4xx: unknown
// item) gets the same answer again.
const audiobookRetries = 1;
const clientErrorStatusMin = 400;
const serverErrorStatusMin = 500;

/** Retry policy of the audiobook queries: network and server errors are retried, once. */
export function shouldRetryAudiobookQuery(failureCount: number, error: Error): boolean {
  const isClientError =
    error instanceof ApiError &&
    error.status >= clientErrorStatusMin &&
    error.status < serverErrorStatusMin;
  return !isClientError && failureCount < audiobookRetries;
}

/**
 * Query options for the authors of a book. Book summaries carry no people, so a card loads the
 * book itself; the entry is the one the book's page shows (BACKLOG M3-14h: authors on summaries).
 */
export function bookAuthorsQueryOptions(client: ApiClient, bookId: string) {
  return queryOptions({
    queryKey: itemQueryKey(bookId),
    queryFn: ({ signal }) =>
      requestData(
        client.GET("/api/v1/items/{itemId}", { params: { path: { itemId: bookId } }, signal }),
      ),
    select: (book) => creditedNames(book.people, "author"),
    retry: shouldRetryAudiobookQuery,
  });
}
