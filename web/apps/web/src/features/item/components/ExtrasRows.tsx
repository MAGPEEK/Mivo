import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { rowCardSizes } from "../../../shared/lib/rowScroll";
import {
  FilmIcon,
  MediaRow,
  Poster,
  stretchedLinkClass,
  type RowCardProps,
} from "../../../shared/ui";
import { useExtras } from "../hooks/useExtras";

/** Props of {@link ExtrasRows}. */
export interface ExtrasRowsProps {
  readonly extras: readonly ItemSummary[];
}

const placeholderIconClass = "size-1/4";

/**
 * The extras of a movie or show, one row per kind with 16:9 cards (DT-08). A card opens the
 * extra's own page, whose Play button starts it in the player. No rows without extras.
 */
export function ExtrasRows({ extras }: ExtrasRowsProps) {
  const rows = useExtras(extras);
  return (
    <>
      {rows.map((row) => (
        <MediaRow
          key={row.type}
          title={row.title}
          items={row.extras}
          format="episode"
          getKey={(extra) => extra.id}
          renderCard={(extra, card) => <ExtraCard extra={extra} card={card} />}
        />
      ))}
    </>
  );
}

function ExtraCard({ extra, card }: { readonly extra: ItemSummary; readonly card: RowCardProps }) {
  return (
    <div className="group relative flex flex-col gap-2">
      <div className="overflow-hidden rounded-md shadow-card">
        <Poster
          imageRef={extra.thumb}
          aspect="still"
          alt=""
          sizes={rowCardSizes("episode")}
          icon={<FilmIcon className={placeholderIconClass} />}
        />
      </div>
      <h3 className="text-sm font-medium break-words">
        <Link
          to="/items/$itemId"
          params={{ itemId: extra.id }}
          {...card}
          className={stretchedLinkClass}
        >
          {extra.title}
        </Link>
      </h3>
    </div>
  );
}
