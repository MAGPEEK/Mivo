import type { LetterIndex } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { libraryLettersQueryOptions } from "../api/libraryQueries";
import type { BrowseListQuery } from "../lib/browseQuery";
import { lettersQueryOf } from "../lib/letterOffsets";

/**
 * Where each first letter starts in the listing `listQuery` (LB-29). `undefined` while it loads
 * and whenever the A-Z bar does not apply: another sort than title, the unmatched filter, a
 * server that cannot index letters yet or a failed request. The bar then stays away, and the
 * listing itself is unaffected.
 */
export function useLibraryLetters(
  libraryId: string,
  listQuery: BrowseListQuery | undefined,
): LetterIndex | undefined {
  const client = useApiClient();
  const lettersQuery = listQuery === undefined ? undefined : lettersQueryOf(listQuery);
  const query = useQuery({
    ...libraryLettersQueryOptions(client, libraryId, lettersQuery ?? {}),
    enabled: lettersQuery !== undefined,
  });
  return lettersQuery === undefined ? undefined : query.data;
}
