/** One part file of an audiobook, as the item detail lists it. */
export interface BookPart {
  readonly id: string;
  /** Probed length in seconds; `undefined` when unknown. */
  readonly durationSeconds?: number | undefined;
}

/** One chapter of an audiobook; the offsets are milliseconds within the file `fileId`. */
export interface BookChapter {
  readonly title: string;
  readonly startMs: number;
  readonly endMs: number;
  readonly fileId: string;
}

/** A chapter placed on the book's timeline, with where the player has to start it. */
export interface TimelineChapter {
  readonly title: string;
  /** The part file that holds the chapter; the player opens this file. */
  readonly fileId: string;
  /** Start within that file in seconds: the player's start position. */
  readonly fileStartSeconds: number;
  /** Start counted from the beginning of the book, across all earlier parts, in seconds. */
  readonly bookStartSeconds: number;
  readonly durationSeconds: number;
}

/** The chapters of a book on one continuous timeline, and the book's total length. */
export interface BookTimeline {
  readonly chapters: readonly TimelineChapter[];
  readonly totalSeconds: number;
}

/** A position inside one part file. */
export interface FilePosition {
  readonly fileId: string;
  readonly seconds: number;
}

const millisecondsPerSecond = 1_000;

/**
 * The length of every part: its probed duration, or the end of its last chapter when that is
 * later or the duration is unknown. The server's chapter reader does the same, so the book
 * timeline never ends before its last chapter (BACKLOG m3-14g).
 */
function partLengths(
  parts: readonly BookPart[],
  chapters: readonly BookChapter[],
): Map<string, number> {
  const lengths = new Map<string, number>();
  for (const part of parts) {
    const duration = part.durationSeconds;
    lengths.set(part.id, duration !== undefined && Number.isFinite(duration) ? duration : 0);
  }
  for (const chapter of chapters) {
    const end = chapter.endMs / millisecondsPerSecond;
    lengths.set(chapter.fileId, Math.max(lengths.get(chapter.fileId) ?? 0, end));
  }
  return lengths;
}

/**
 * The play order of the parts: the order the chapters (which come in playback order) visit
 * them, then the parts without a chapter in file order.
 */
function partOrder(parts: readonly BookPart[], chapters: readonly BookChapter[]): string[] {
  const order = [...new Set(chapters.map((chapter) => chapter.fileId))];
  for (const part of parts) {
    if (!order.includes(part.id)) {
      order.push(part.id);
    }
  }
  return order;
}

/** Where every part starts on the book's timeline, and the length of the whole book. */
function layOutParts(
  parts: readonly BookPart[],
  chapters: readonly BookChapter[],
): { readonly partStarts: Map<string, number>; readonly totalSeconds: number } {
  const lengths = partLengths(parts, chapters);
  const partStarts = new Map<string, number>();
  let totalSeconds = 0;
  for (const partId of partOrder(parts, chapters)) {
    partStarts.set(partId, totalSeconds);
    totalSeconds += lengths.get(partId) ?? 0;
  }
  return { partStarts, totalSeconds };
}

/**
 * Where the part file `fileId` starts on the book's timeline in seconds: the length of the
 * parts played before it, 0 for the first part. A player adds it to the position in the file
 * to report the position in the book (API `ProgressReport`). `undefined` for a file that is
 * not a part of the book.
 */
export function bookPartStart(
  parts: readonly BookPart[],
  chapters: readonly BookChapter[],
  fileId: string,
): number | undefined {
  return layOutParts(parts, chapters).partStarts.get(fileId);
}

/**
 * Places the chapters of a book on one timeline. The server sends chapter offsets relative to
 * their part file (API `ItemChapter`); a book of several parts starts each part where the
 * previous one ends, so a chapter's book offset is the length of the earlier parts plus its
 * offset in its own file. The player still plays one file, so each chapter keeps its file and
 * file offset as the start target.
 */
export function bookTimeline(
  parts: readonly BookPart[],
  chapters: readonly BookChapter[],
): BookTimeline {
  const { partStarts, totalSeconds } = layOutParts(parts, chapters);
  return {
    chapters: chapters.map((chapter) => {
      const fileStartSeconds = chapter.startMs / millisecondsPerSecond;
      return {
        title: chapter.title,
        fileId: chapter.fileId,
        fileStartSeconds,
        bookStartSeconds: (partStarts.get(chapter.fileId) ?? 0) + fileStartSeconds,
        durationSeconds: Math.max(0, (chapter.endMs - chapter.startMs) / millisecondsPerSecond),
      };
    }),
    totalSeconds,
  };
}

/**
 * Maps a position on the book's timeline to the chapter's part file and the offset within it,
 * e.g. to resume a book. A position before the first chapter maps to the start of the first
 * one, a position after the end of the book to its end; `undefined` for a book without
 * chapters.
 */
export function filePositionAt(
  timeline: BookTimeline,
  bookSeconds: number,
): FilePosition | undefined {
  const { chapters } = timeline;
  let found: TimelineChapter | undefined;
  for (const chapter of chapters) {
    if (chapter.bookStartSeconds > bookSeconds) {
      break;
    }
    found = chapter;
  }
  const chapter = found ?? chapters[0];
  if (chapter === undefined) {
    return undefined;
  }
  const clamped = Math.min(bookSeconds, timeline.totalSeconds);
  const intoChapter = Math.max(0, clamped - chapter.bookStartSeconds);
  return { fileId: chapter.fileId, seconds: chapter.fileStartSeconds + intoChapter };
}
