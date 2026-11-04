import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { FilmIcon, Poster, ScreenIcon, Skeleton, stretchedLinkClass } from "../../../shared/ui";
import { useItemChildren, type ItemChildren } from "../hooks/useItemChildren";
import type { ChildRow } from "../lib/childRow";
import { oppositeOrder, searchForOrder, type ChildOrder } from "../lib/itemSearch";
import { itemNamespace } from "../locales";

/** Props of {@link ChildrenList}. */
export interface ChildrenListProps {
  /** The show or season whose children are listed. */
  readonly itemId: string;
  /** `season` lists the children of a show (seasons), `episode` those of a season. */
  readonly childKind: "season" | "episode";
  readonly order: ChildOrder;
}

const buttonClass =
  "rounded-md border border-divider bg-surface px-4 py-2 text-sm font-medium text-text hover:bg-surface-raised aria-disabled:cursor-not-allowed aria-disabled:text-text-muted";
const seasonGridClass =
  "grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8";
const seasonPosterSizes =
  "(min-width: 1280px) 12.5vw, (min-width: 1024px) 17vw, (min-width: 768px) 25vw, (min-width: 640px) 34vw, 50vw";
const episodeStillSizes = "10rem";
const placeholderIconClass = "size-1/4";

/**
 * The seasons of a show as a poster grid, or the episodes of a season as a list with 16:9
 * stills, page by page. Only the episode list can be reversed.
 */
export function ChildrenList({ itemId, childKind, order }: ChildrenListProps) {
  const { t } = useTranslation(itemNamespace);
  const headingId = useId();
  const list = useItemChildren(itemId, order);
  const isEpisodes = childKind === "episode";
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-4">
        <h2 id={headingId} className="text-lg font-semibold">
          {isEpisodes ? t("episode.list") : t("season.list")}
        </h2>
        {isEpisodes && <OrderToggle itemId={itemId} order={order} />}
      </div>
      <ChildrenBody list={list} childKind={childKind} />
    </section>
  );
}

function OrderToggle({ itemId, order }: { readonly itemId: string; readonly order: ChildOrder }) {
  const { t } = useTranslation(itemNamespace);
  const target = oppositeOrder(order);
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId }}
      search={searchForOrder(target)}
      replace
      className="rounded-sm text-sm text-text-muted underline underline-offset-4 hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
    >
      {target === "desc" ? t("order.toDescending") : t("order.toAscending")}
    </Link>
  );
}

interface ChildrenBodyProps {
  readonly list: ItemChildren;
  readonly childKind: "season" | "episode";
}

function ChildrenBody({ list, childKind }: ChildrenBodyProps) {
  const { t } = useTranslation(itemNamespace);
  if (list.status === "pending") {
    return (
      <div role="status" className="w-40">
        <Skeleton label={t("children.loading")} className="aspect-video w-full rounded-md" />
      </div>
    );
  }
  if (list.status === "error") {
    return (
      <div className="flex flex-wrap items-center gap-4">
        <p role="alert" className="text-text-muted">
          {t("children.loadFailed")}
        </p>
        <button type="button" onClick={list.retry} className={buttonClass}>
          {t("children.retry")}
        </button>
      </div>
    );
  }
  if (list.rows.length === 0) {
    return <p className="text-text-muted">{t("children.empty")}</p>;
  }
  return (
    <>
      {childKind === "season" ? (
        <ul className={seasonGridClass}>
          {list.rows.map((row) => (
            <li key={row.id}>
              <SeasonCard row={row} />
            </li>
          ))}
        </ul>
      ) : (
        <ul className="flex flex-col gap-4">
          {list.rows.map((row) => (
            <li key={row.id}>
              <EpisodeRow row={row} />
            </li>
          ))}
        </ul>
      )}
      {list.hasMore && <LoadMore list={list} />}
    </>
  );
}

function SeasonCard({ row }: { readonly row: ChildRow }) {
  return (
    <div className="group relative flex flex-col gap-2">
      <Poster
        imageRef={row.image}
        aspect="poster"
        alt=""
        sizes={seasonPosterSizes}
        icon={<ScreenIcon className={placeholderIconClass} />}
        className="rounded-md shadow-card"
      />
      <h3 className="text-sm font-medium">
        <Link to="/items/$itemId" params={{ itemId: row.id }} className={stretchedLinkClass}>
          {row.title}
        </Link>
      </h3>
    </div>
  );
}

function EpisodeRow({ row }: { readonly row: ChildRow }) {
  const details = [row.label, row.runtime].filter((text) => text !== "");
  return (
    <div className="relative flex items-center gap-4">
      <Poster
        imageRef={row.image}
        aspect="still"
        alt=""
        sizes={episodeStillSizes}
        icon={<FilmIcon className={placeholderIconClass} />}
        className="w-40 shrink-0 rounded-md shadow-card"
      />
      <div className="flex min-w-0 flex-col gap-1">
        <h3 className="font-medium break-words">
          <Link to="/items/$itemId" params={{ itemId: row.id }} className={stretchedLinkClass}>
            {row.title}
          </Link>
        </h3>
        {details.length > 0 && <p className="text-sm text-text-muted">{details.join(" · ")}</p>}
      </div>
    </div>
  );
}

function LoadMore({ list }: { readonly list: ItemChildren }) {
  const { t } = useTranslation(itemNamespace);
  let label = list.loadMoreFailed ? t("children.retry") : t("children.loadMore");
  if (list.loadingMore) {
    label = t("children.loadingMore");
  }
  return (
    <div className="mt-8 flex flex-col items-center gap-3">
      {list.loadMoreFailed && !list.loadingMore && (
        <p role="alert" className="text-sm text-text-muted">
          {t("children.loadMoreFailed")}
        </p>
      )}
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        onClick={list.loadMore}
        aria-disabled={list.loadingMore}
        aria-busy={list.loadingMore}
        className={buttonClass}
      >
        {label}
      </button>
    </div>
  );
}
