import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { bookAuthorsQueryOptions } from "../api/audiobookQueries";
import { joinNames } from "../lib/bookCredits";

/**
 * The authors of the book `bookId` as one line, e.g. "Ada Inkwell and Bram Quill";
 * `undefined` while it loads, when it fails or for a book without an author. A card then
 * simply shows no author line.
 */
export function useBookAuthors(bookId: string): string | undefined {
  const client = useApiClient();
  const { i18n } = useTranslation();
  const query = useQuery(bookAuthorsQueryOptions(client, bookId));
  const authors = query.data;
  return authors === undefined || authors.length === 0
    ? undefined
    : joinNames(authors, i18n.language);
}
