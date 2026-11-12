import { Link } from "@tanstack/react-router";

import { FilmIcon, Poster, stretchedLinkClass } from "../../../shared/ui";
import { WatchBadgeOverlay, cardSize, type ViewStyle } from "../../library";
import type { ChildRow } from "../lib/childRow";

/** Props of {@link EpisodeGrid}. */
export interface EpisodeGridProps {
  readonly rows: readonly ChildRow[];
  readonly viewStyle: ViewStyle;
  /** Slider step of the grid; the other views ignore it. */
  readonly cardStep: number;
}

const placeholderIconClass = "size-1/4";
const detailsStillSizes = "10rem";
const detailsRowClass = "relative flex items-center gap-4";
const listRowClass =
  "relative grid grid-cols-[1fr_auto] items-center gap-3 rounded-sm border-b border-divider px-2.5 py-2 hover:bg-surface focus-within:bg-surface";

function detailsOf(row: ChildRow): string {
  return [row.label, row.runtime].filter((text) => text !== "").join(" · ");
}

function EpisodeLink({ row }: { readonly row: ChildRow }) {
  return (
    <Link to="/items/$itemId" params={{ itemId: row.id }} className={stretchedLinkClass}>
      {row.title}
    </Link>
  );
}

function EpisodeStill({ row, sizes }: { readonly row: ChildRow; readonly sizes: string }) {
  return (
    <div className="relative overflow-hidden rounded-md shadow-card">
      <Poster
        imageRef={row.image}
        aspect="still"
        alt=""
        sizes={sizes}
        icon={<FilmIcon className={placeholderIconClass} />}
      />
      <WatchBadgeOverlay badges={row.watch} />
    </div>
  );
}

function EpisodeCard({ row, sizes }: { readonly row: ChildRow; readonly sizes: string }) {
  const details = detailsOf(row);
  return (
    <div className="group relative flex flex-col gap-2">
      <EpisodeStill row={row} sizes={sizes} />
      <div className="flex min-w-0 flex-col gap-0.5">
        <h3 className="text-sm font-medium break-words">
          <EpisodeLink row={row} />
        </h3>
        {details !== "" && <p className="text-xs text-text-muted">{details}</p>}
      </div>
    </div>
  );
}

function DetailsRow({ row }: { readonly row: ChildRow }) {
  const details = detailsOf(row);
  return (
    <div className={detailsRowClass}>
      <div className="w-40 shrink-0">
        <EpisodeStill row={row} sizes={detailsStillSizes} />
      </div>
      <div className="flex min-w-0 flex-col gap-1">
        <h3 className="font-medium break-words">
          <EpisodeLink row={row} />
        </h3>
        {details !== "" && <p className="text-sm text-text-muted">{details}</p>}
      </div>
    </div>
  );
}

function CompactRow({ row }: { readonly row: ChildRow }) {
  return (
    <div className={listRowClass}>
      <h3 className="min-w-0 font-medium break-words">
        <EpisodeLink row={row} />
      </h3>
      <p className="text-sm text-text-muted">{detailsOf(row)}</p>
    </div>
  );
}

/**
 * The episodes of a season in the device's view style: 16:9 cards in a size-slider grid, compact
 * rows, or rows with a still. The rows are the display content, so no view knows the API.
 */
export function EpisodeGrid({ rows, viewStyle, cardStep }: EpisodeGridProps) {
  if (viewStyle === "grid") {
    const size = cardSize(cardStep, "episode");
    return (
      // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
      <ul
        role="list"
        className="grid gap-x-4 gap-y-6"
        style={{
          gridTemplateColumns: `repeat(auto-fill, minmax(${String(size.columnWidth)}px, 1fr))`,
        }}
      >
        {rows.map((row) => (
          <li key={row.id}>
            <EpisodeCard row={row} sizes={size.sizes} />
          </li>
        ))}
      </ul>
    );
  }
  return (
    // eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets
    <ul role="list" className={viewStyle === "details" ? "flex flex-col gap-4" : undefined}>
      {rows.map((row) => (
        <li key={row.id}>
          {viewStyle === "details" ? <DetailsRow row={row} /> : <CompactRow row={row} />}
        </li>
      ))}
    </ul>
  );
}
