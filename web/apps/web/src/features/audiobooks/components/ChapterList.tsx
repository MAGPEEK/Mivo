import type { ItemDetail } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { Badge, PlayIcon, stretchedLinkClass } from "../../../shared/ui";
import { useChapterRows } from "../hooks/useChapterRows";
import type { ChapterRowModel } from "../lib/chapterRows";
import { audiobooksNamespace } from "../locales";

/** Props of {@link ChapterList}. */
export interface ChapterListProps {
  /** The loaded book with its files and chapters. */
  readonly book: ItemDetail;
}

/**
 * The chapters of a book in playback order with their start in the book and their length. A
 * row starts its chapter in the player: the part file that holds it, at its offset in that file.
 */
export function ChapterList({ book }: ChapterListProps) {
  const { t } = useTranslation(audiobooksNamespace);
  const headingId = useId();
  const rows = useChapterRows(book);
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="mb-4 text-lg font-semibold">
        {t("chapters.label")}
      </h2>
      {rows.length === 0 ? (
        <p className="text-text-muted">{t("chapters.empty")}</p>
      ) : (
        <ol aria-labelledby={headingId} className="flex flex-col">
          {rows.map((row) => (
            <li key={row.number}>
              <ChapterRow bookId={book.id} row={row} />
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

interface ChapterRowProps {
  readonly bookId: string;
  readonly row: ChapterRowModel;
}

function ChapterRow({ bookId, row }: ChapterRowProps) {
  const { t } = useTranslation(audiobooksNamespace);
  return (
    <div className="group relative grid grid-cols-[2.75rem_1fr_auto] items-center gap-3 rounded-sm border-b border-divider px-2 py-2 hover:bg-surface focus-within:bg-surface">
      <span className="relative grid h-8 place-items-center text-sm text-text-muted tabular-nums">
        <span className={row.available ? "group-focus-within:invisible group-hover:invisible" : ""}>
          {row.number}
        </span>
        {row.available && (
          <PlayIcon className="invisible absolute size-5 text-text group-focus-within:visible group-hover:visible" />
        )}
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="flex min-w-0 items-center gap-2">
          {row.available ? (
            <Link
              to="/items/$itemId/play"
              params={{ itemId: bookId }}
              search={{ fileId: row.fileId, t: row.fileStartSeconds }}
              aria-label={t("chapters.play", { title: row.title })}
              className={`truncate font-medium ${stretchedLinkClass}`}
            >
              {row.title}
            </Link>
          ) : (
            <>
              <span className="truncate font-medium text-text-muted">{row.title}</span>
              <Badge>{t("chapters.unavailable")}</Badge>
            </>
          )}
        </span>
        <span className="text-sm text-text-muted tabular-nums">
          {t("chapters.startsAt", { time: row.start })}
        </span>
      </span>
      <span className="text-sm text-text-muted tabular-nums">
        <span className="sr-only">{t("chapters.length")} </span>
        {row.duration}
      </span>
    </div>
  );
}
