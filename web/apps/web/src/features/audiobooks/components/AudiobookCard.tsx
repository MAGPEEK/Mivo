import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { MediaCard, Poster } from "../../../shared/ui";
import { formatDuration, itemPosterSizes, LibraryKindIcon, useFocusOnMount } from "../../library";
import { useBookAuthors } from "../hooks/useBookAuthors";

/** Props of {@link AudiobookCard}. */
export interface AudiobookCardProps {
  readonly book: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

/** Card of one book in the grid: 2:3 cover, title, author and length; opens the book. */
export function AudiobookCard({ book, focusOnMount = false }: AudiobookCardProps) {
  const authors = useBookAuthors(book.id);
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  // The summary's length of a book of several parts is the sum of its parts.
  const seconds = book.media?.durationSeconds;
  return (
    <MediaCard
      title={book.title}
      poster={
        <Poster
          imageRef={book.poster}
          aspect="poster"
          alt={book.title}
          sizes={itemPosterSizes}
          icon={<LibraryKindIcon kind="audiobooks" className="size-1/4" />}
        />
      }
      muted={book.media?.available === false}
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: book.id }} ref={linkRef} {...linkProps} />
      )}
      details={
        <>
          {authors !== undefined && <span className="truncate">{authors}</span>}
          {seconds !== undefined && <span>{formatDuration(seconds)}</span>}
        </>
      }
    />
  );
}
