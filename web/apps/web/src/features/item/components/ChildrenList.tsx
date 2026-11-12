import { Link } from "@tanstack/react-router";
import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { rowCardSizes } from "../../../shared/lib/rowScroll";
import {
  MediaRow,
  Poster,
  ScreenIcon,
  Skeleton,
  stretchedLinkClass,
  type RowCardProps,
} from "../../../shared/ui";
import { ViewControls, WatchBadgeOverlay, useViewPreference } from "../../library";
import { useItemChildren, type ItemChildren } from "../hooks/useItemChildren";
import type { ChildRow } from "../lib/childRow";
import { oppositeOrder, searchForOrder, type ChildOrder } from "../lib/itemSearch";
import { itemNamespace } from "../locales";
import { EpisodeGrid } from "./EpisodeGrid";

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
const placeholderIconClass = "size-1/4";

/**
 * The seasons of a show as a scrolling poster row, or the episodes of a season in the device's
 * view style, page by page. Only the episode list can be reversed.
 */
export function ChildrenList({ itemId, childKind, order }: ChildrenListProps) {
  return childKind === "season" ? (
    <SeasonsSection itemId={itemId} />
  ) : (
    <EpisodesSection itemId={itemId} order={order} />
  );
}

function SeasonsSection({ itemId }: { readonly itemId: string }) {
  const { t } = useTranslation(itemNamespace);
  const list = useItemChildren(itemId, "asc");
  return (
    <ChildrenFrame list={list} heading={t("season.list")} hasToolbar={false}>
      {(rows) => (
        <MediaRow
          title={t("season.list")}
          items={rows}
          format="poster"
          getKey={(row) => row.id}
          renderCard={(row, card) => <SeasonCard row={row} card={card} />}
        />
      )}
    </ChildrenFrame>
  );
}

function EpisodesSection({
  itemId,
  order,
}: {
  readonly itemId: string;
  readonly order: ChildOrder;
}) {
  const { t } = useTranslation(itemNamespace);
  const list = useItemChildren(itemId, order);
  const view = useViewPreference();
  return (
    <ChildrenFrame
      list={list}
      heading={t("episode.list")}
      hasToolbar
      toolbar={
        <>
          <OrderToggle itemId={itemId} order={order} />
          <ViewControls view={view} />
        </>
      }
    >
      {(rows) => <EpisodeGrid rows={rows} viewStyle={view.viewStyle} cardStep={view.cardStep} />}
    </ChildrenFrame>
  );
}

interface ChildrenFrameProps {
  readonly list: ItemChildren;
  readonly heading: string;
  /** Whether the section draws its own heading row; a {@link MediaRow} brings its own. */
  readonly hasToolbar: boolean;
  readonly toolbar?: ReactNode;
  readonly children: (rows: readonly ChildRow[]) => ReactNode;
}

// Loading, error and empty states and "Load more" around the loaded rows.
function ChildrenFrame({ list, heading, hasToolbar, toolbar, children }: ChildrenFrameProps) {
  const headingId = useId();
  const loaded = list.status === "success" && list.rows.length > 0;
  if (loaded && !hasToolbar) {
    return (
      <div className="mt-8">
        {children(list.rows)}
        {list.hasMore && <LoadMore list={list} />}
      </div>
    );
  }
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-4">
        <h2 id={headingId} className="text-lg font-semibold">
          {heading}
        </h2>
        {hasToolbar && <div className="flex flex-wrap items-center gap-3">{toolbar}</div>}
      </div>
      {loaded ? children(list.rows) : <ChildrenStatus list={list} />}
      {loaded && list.hasMore && <LoadMore list={list} />}
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

function ChildrenStatus({ list }: { readonly list: ItemChildren }) {
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
  return <p className="text-text-muted">{t("children.empty")}</p>;
}

function SeasonCard({ row, card }: { readonly row: ChildRow; readonly card: RowCardProps }) {
  return (
    <div className="group relative flex flex-col gap-2">
      <div className="relative overflow-hidden rounded-md shadow-card">
        <Poster
          imageRef={row.image}
          aspect="poster"
          alt=""
          sizes={rowCardSizes("poster")}
          icon={<ScreenIcon className={placeholderIconClass} />}
        />
        <WatchBadgeOverlay badges={row.watch} />
      </div>
      <h3 className="text-sm font-medium">
        <Link
          to="/items/$itemId"
          params={{ itemId: row.id }}
          {...card}
          className={stretchedLinkClass}
        >
          {row.title}
        </Link>
      </h3>
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
