import type { ItemDetail } from "@mivo/api-client";
import { bookTimeline } from "@mivo/player-core";

import { formatDuration } from "../../library";

/** What one row of a book's chapter list shows, and where Play starts the chapter. */
export interface ChapterRowModel {
  /** Position in the book, from 1. */
  readonly number: number;
  readonly title: string;
  /** Start counted from the beginning of the book in clock notation, e.g. `1:02:05`. */
  readonly start: string;
  /** Length in clock notation, e.g. `12:40`. */
  readonly duration: string;
  /** The part file the player opens. */
  readonly fileId: string;
  /** Start within that file in seconds; `undefined` for a chapter at the start of its file. */
  readonly fileStartSeconds: number | undefined;
  /** False when the chapter's file cannot be played: the row offers no Play. */
  readonly available: boolean;
}

/**
 * The chapter rows of a book. Their start is counted across all parts (what a listener reads),
 * while Play starts the chapter's own part file at its offset in that file.
 */
export function chapterRows(book: ItemDetail): ChapterRowModel[] {
  const availableFiles = new Set(
    book.files.filter((file) => file.available).map((file) => file.id),
  );
  const timeline = bookTimeline(book.files, book.chapters ?? []);
  return timeline.chapters.map((chapter, index) => ({
    number: index + 1,
    title: chapter.title,
    start: formatDuration(chapter.bookStartSeconds),
    duration: formatDuration(chapter.durationSeconds),
    fileId: chapter.fileId,
    fileStartSeconds: chapter.fileStartSeconds > 0 ? chapter.fileStartSeconds : undefined,
    available: availableFiles.has(chapter.fileId),
  }));
}
