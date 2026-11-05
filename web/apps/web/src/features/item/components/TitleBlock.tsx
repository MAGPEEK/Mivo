import { Link } from "@tanstack/react-router";
import { useId, useState, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { genreBrowseSearch, shownGenres, visibleGenreCount } from "../lib/genreLink";
import type { ItemView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/** Props of {@link TitleBlock}. */
export interface TitleBlockProps {
  /** Receives the title heading, so focus can move to it after a retry or navigation. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly item: ItemView;
  /** Shown above the title, e.g. the links to the show and season. */
  readonly above?: ReactNode;
  /** Facts that come before the item's own, e.g. the `S1 · E3` label. */
  readonly leadFacts?: readonly string[];
}

const factSeparator = " · ";
const linkClass =
  "rounded-sm text-text underline decoration-dotted underline-offset-4 hover:decoration-solid focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * Title, original title, content rating, facts (year, runtime, counts), genres and director
 * (DT-03). A missing field hides its whole line, so no empty separator or "undefined" shows.
 */
export function TitleBlock({ headingRef, item, above, leadFacts = [] }: TitleBlockProps) {
  const { t } = useTranslation(itemNamespace);
  const facts = [...leadFacts, ...item.facts];
  const hasMeta = item.contentRating !== undefined || facts.length > 0 || item.genres.length > 0;
  return (
    <div className="flex min-w-0 flex-col gap-2">
      {above}
      <h1
        ref={headingRef}
        tabIndex={-1}
        className="text-4xl font-semibold break-words focus-visible:outline-none"
      >
        {item.title}
      </h1>
      {item.originalTitle !== undefined && (
        <p className="text-text-muted">
          {t("titleBlock.originalTitle", { title: item.originalTitle })}
        </p>
      )}
      {hasMeta && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          {item.contentRating !== undefined && <RatingBadge rating={item.contentRating} />}
          {facts.length > 0 && <p>{facts.join(factSeparator)}</p>}
          <GenreLinks genres={item.genres} libraryId={item.libraryId} />
        </div>
      )}
      {item.directorLine !== undefined && <p className="text-text-muted">{item.directorLine}</p>}
    </div>
  );
}

function RatingBadge({ rating }: { readonly rating: string }) {
  const { t } = useTranslation(itemNamespace);
  return (
    <p className="rounded-sm border border-text-muted px-1.5 text-xs leading-5 font-bold">
      <span className="sr-only">{t("titleBlock.contentRating")}: </span>
      {rating}
    </p>
  );
}

interface GenreLinksProps {
  readonly genres: readonly string[];
  readonly libraryId: string;
}

// The first three genres link to Browse filtered by that genre; "+n" shows the rest.
function GenreLinks({ genres, libraryId }: GenreLinksProps) {
  const { t } = useTranslation(itemNamespace);
  const listId = useId();
  const [expanded, setExpanded] = useState(false);
  if (genres.length === 0) {
    return null;
  }
  const hidden = genres.length - visibleGenreCount;
  return (
    <div className="flex flex-wrap items-center gap-x-2">
      <ul id={listId} aria-label={t("genres")} className="flex flex-wrap gap-x-2">
        {shownGenres(genres, expanded).map((genre) => (
          <li key={genre}>
            <Link
              to="/libraries/$libraryId"
              params={{ libraryId }}
              search={genreBrowseSearch(genre)}
              className={linkClass}
            >
              {genre}
            </Link>
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={listId}
          // "+2" alone does not say what it adds; the name starts with the visible text.
          aria-label={expanded ? undefined : t("titleBlock.moreGenres", { count: hidden })}
          onClick={() => {
            setExpanded((open) => !open);
          }}
          className="rounded-sm text-sm font-medium text-accent hover:text-accent-hover hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        >
          {expanded
            ? t("titleBlock.fewerGenres")
            : t("titleBlock.moreGenresShort", { count: hidden })}
        </button>
      )}
    </div>
  );
}
