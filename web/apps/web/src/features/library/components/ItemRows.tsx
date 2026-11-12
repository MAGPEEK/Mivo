import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AlertIcon, Badge, FilmIcon, Poster, ScreenIcon } from "../../../shared/ui";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import { toItemRowModel, type ItemRowModel } from "../lib/itemRow";
import { libraryNamespace } from "../locales";

/** Props shared by {@link ListView} and {@link DetailsView}. Views know nothing of libraries. */
export interface ItemRowsProps {
  readonly items: readonly ItemSummary[];
  /** Accessible name of the list. */
  readonly label: string;
  /** Id of the item whose link takes the focus when it mounts (first row of a new page). */
  readonly focusItemId?: string | undefined;
}

const rowLinkClass = "font-semibold hover:underline";
const mutedRowLinkClass = "font-semibold text-text-muted hover:underline";
const rowBorderClass =
  "border-b border-divider rounded-sm hover:bg-surface focus-within:bg-surface";
/** Classes of a List row's `li`; the virtual grid adds the position. */
export const listRowClass = `${rowBorderClass} grid grid-cols-[1fr_4rem_auto] items-center gap-3 px-2.5 py-2 sm:grid-cols-[1fr_4rem_6rem_auto]`;
/** Classes of a Details row's `li`. */
export const detailsRowClass = `${rowBorderClass} flex items-start gap-4 p-2.5`;
// Posters of the Details view are a fixed 72 px wide thumbnail.
const thumbnailWidth = 72;
const thumbnailSizes = `${String(thumbnailWidth)}px`;

interface RowTitleProps {
  readonly row: ItemRowModel;
  readonly focusOnMount: boolean;
}

function RowTitle({ row, focusOnMount }: RowTitleProps) {
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: row.id }}
      ref={linkRef}
      className={row.available ? rowLinkClass : mutedRowLinkClass}
    >
      {row.title}
    </Link>
  );
}

/** Props of {@link ListRowBody} and {@link DetailsRowBody}: the content of one row. */
export interface RowBodyProps {
  readonly item: ItemSummary;
  readonly focusOnMount: boolean;
}

function UnavailableBadge() {
  const { t } = useTranslation(libraryNamespace);
  return (
    <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
      {t("items.unavailable")}
    </Badge>
  );
}

/** The cells of a compact row (LB-27): title, year, duration and the resolution chip. */
export function ListRowBody({ item, focusOnMount }: RowBodyProps) {
  const row = toItemRowModel(item);
  return (
    <>
      <RowTitle row={row} focusOnMount={focusOnMount} />
      <span className="text-text-muted">{row.year}</span>
      <span className="hidden text-text-muted sm:inline">{row.duration}</span>
      <span className="justify-self-end">
        {row.available ? (
          row.resolution !== "" && <Badge>{row.resolution}</Badge>
        ) : (
          <UnavailableBadge />
        )}
      </span>
    </>
  );
}

/** Compact rows (LB-27): title, year, duration and the resolution chip. */
export function ListView({ items, label, focusItemId }: ItemRowsProps) {
  return (
    // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
    <ul role="list" aria-label={label}>
      {items.map((item) => (
        <li key={item.id} className={listRowClass}>
          <ListRowBody item={item} focusOnMount={item.id === focusItemId} />
        </li>
      ))}
    </ul>
  );
}

/** Rows with a poster thumbnail, a summary line and technical chips (LB-27). */
export function DetailsView({ items, label, focusItemId }: ItemRowsProps) {
  return (
    // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
    <ul role="list" aria-label={label}>
      {items.map((item) => (
        <li key={item.id} className={detailsRowClass}>
          <DetailsRowBody item={item} focusOnMount={item.id === focusItemId} />
        </li>
      ))}
    </ul>
  );
}

/** The content of a Details row: thumbnail, title, summary line and chips. */
export function DetailsRowBody({ item, focusOnMount }: RowBodyProps) {
  const { t } = useTranslation(libraryNamespace);
  const row = toItemRowModel(item);
  const summary = detailsSummary(row, t("items.seasons", { count: row.seasonCount }));
  const chips = [row.chip, row.resolution].filter((chip) => chip !== "");
  return (
    <>
      <div className="shrink-0" style={{ width: thumbnailWidth }}>
        <Poster
          imageRef={item.poster}
          aspect="poster"
          alt=""
          sizes={thumbnailSizes}
          icon={
            row.isShow ? <ScreenIcon className="size-1/3" /> : <FilmIcon className="size-1/3" />
          }
        />
      </div>
      <div className="min-w-0">
        <h2 className="text-[0.9375rem]">
          <RowTitle row={row} focusOnMount={focusOnMount} />
        </h2>
        {summary !== "" && <p className="my-1 text-sm text-text-muted">{summary}</p>}
        <div className="flex flex-wrap gap-1">
          {chips.map((chip) => (
            <Badge key={chip}>{chip}</Badge>
          ))}
          {!row.available && <UnavailableBadge />}
          {row.unmatched && <Badge>{t("items.unmatched")}</Badge>}
        </div>
      </div>
    </>
  );
}

const summarySeparator = " · ";

// Year and runtime, or the season count of a show: what the item summary knows of the story.
function detailsSummary(row: ItemRowModel, seasons: string): string {
  return [row.year, row.isShow ? seasons : row.duration]
    .filter((part) => part !== "")
    .join(summarySeparator);
}
