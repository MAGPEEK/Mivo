import type { ItemDetail } from "@mivo/api-client";
import { useMemo } from "react";

import { chapterRows, type ChapterRowModel } from "../lib/chapterRows";

/** The chapter rows of a loaded book; recomputed only when the book changes. */
export function useChapterRows(book: ItemDetail): readonly ChapterRowModel[] {
  return useMemo(() => chapterRows(book), [book]);
}
