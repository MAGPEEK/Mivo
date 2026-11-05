import { useId, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { cardWidths, type CardFormat, type ScrollDirection } from "../lib/rowScroll";
import { ChevronLeftIcon, ChevronRightIcon } from "./icons";
import { useMediaRow, type RowCardProps, type RowSlot } from "./useMediaRow";

export type { RowCardProps } from "./useMediaRow";

/** What {@link MediaRowProps.renderSeeAll} receives; spread it onto the link element. */
export interface SeeAllLinkProps {
  readonly className: string;
  readonly children: ReactNode;
  readonly "aria-label": string;
}

/** Props of {@link MediaRow}. */
export interface MediaRowProps<Item> {
  /** Row heading, e.g. "Recently added in Movies"; also names the section and the list. */
  readonly title: string;
  readonly items: readonly Item[];
  /** Shape of every card in the row; sets the card width. */
  readonly format: CardFormat;
  readonly getKey: (item: Item) => string;
  /** Renders one card, e.g. an `ItemCard`; spread `card` onto the card's link. */
  readonly renderCard: (item: Item, card: RowCardProps) => ReactNode;
  /** Renders the "See all" link with the given props, e.g. a router `Link`; omitted without one. */
  readonly renderSeeAll?: (props: SeeAllLinkProps) => ReactNode;
}

const seeAllClass =
  "rounded-sm text-sm text-text-muted hover:text-accent-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

// The buttons appear while the pointer is over the row or focus is inside it, and vanish at
// the end they point to. Touch screens swipe instead.
const arrowClass =
  "absolute top-1/2 z-10 grid h-16 w-9 -translate-y-1/2 place-items-center rounded-md border border-divider bg-surface/90 text-text opacity-0 transition-opacity focus-visible:outline-2 focus-visible:outline-accent motion-reduce:transition-none pointer-coarse:hidden";
const activeArrowClass =
  "hover:bg-surface-raised focus-visible:opacity-100 group-hover/row:opacity-100 group-focus-within/row:opacity-100";
// A button at its end keeps focus (it is aria-disabled, not disabled, so focus does not fall
// back to the page) and stays dimly visible while focused; otherwise it is hidden.
const endArrowClass = "pointer-events-none focus-visible:opacity-50";

interface ArrowButtonProps {
  readonly direction: ScrollDirection;
  readonly label: string;
  readonly atEnd: boolean;
  readonly onScroll: (direction: ScrollDirection) => void;
}

function ArrowButton({ direction, label, atEnd, onScroll }: ArrowButtonProps) {
  const back = direction < 0;
  return (
    <button
      type="button"
      aria-label={label}
      aria-disabled={atEnd}
      onClick={() => {
        onScroll(direction);
      }}
      className={`${arrowClass} ${atEnd ? endArrowClass : activeArrowClass} ${back ? "-left-3" : "-right-3"}`}
    >
      {back ? <ChevronLeftIcon /> : <ChevronRightIcon />}
    </button>
  );
}

interface RowHeaderProps {
  readonly headingId: string;
  readonly title: string;
  readonly renderSeeAll: MediaRowProps<unknown>["renderSeeAll"];
}

function RowHeader({ headingId, title, renderSeeAll }: RowHeaderProps) {
  const { t } = useTranslation();
  return (
    <div className="mb-2 flex items-baseline gap-3">
      <h2 id={headingId} className="text-lg font-semibold text-text">
        {title}
      </h2>
      {renderSeeAll?.({
        className: seeAllClass,
        children: (
          <>
            {t("mediaRow.seeAll")} <span aria-hidden="true">›</span>
          </>
        ),
        "aria-label": t("mediaRow.seeAllLabel", { title }),
      })}
    </div>
  );
}

interface RowTrackProps<Item> extends Pick<
  MediaRowProps<Item>,
  "items" | "format" | "getKey" | "renderCard"
> {
  readonly scrollRef: RefObject<HTMLDivElement | null>;
  readonly listRef: RefObject<HTMLUListElement | null>;
  readonly slots: readonly RowSlot[];
  readonly totalWidth: number;
  readonly cardProps: (index: number) => RowCardProps;
}

// Every card sits in the same grid cell, moved along by its offset: the list takes the height
// of its cards although only some of them are rendered.
function RowTrack<Item>({
  items,
  format,
  getKey,
  renderCard,
  scrollRef,
  listRef,
  slots,
  totalWidth,
  cardProps,
}: RowTrackProps<Item>) {
  return (
    // Room around the cards keeps their hover lift and focus ring inside the scroll box.
    <div
      ref={scrollRef}
      data-row-scroller=""
      className="overflow-x-auto px-1 py-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {/* eslint-disable-next-line jsx-a11y/no-redundant-roles -- Safari drops the list role of a list without bullets */}
      <ul ref={listRef} role="list" className="grid" style={{ width: totalWidth }}>
        {slots.map(({ index, start }) => {
          const item = items[index];
          return item === undefined ? null : (
            <li
              key={getKey(item)}
              data-index={index}
              aria-setsize={items.length}
              aria-posinset={index + 1}
              className="col-start-1 row-start-1"
              style={{ width: cardWidths[format], transform: `translateX(${String(start)}px)` }}
            >
              {renderCard(item, cardProps(index))}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/**
 * One horizontally scrolling row of cards with a heading, an optional "See all" link and
 * arrow buttons (HO-02). Long rows render only the cards in view; arrow keys move between
 * cards, Home and End jump to the ends.
 */
export function MediaRow<Item>({ title, renderSeeAll, ...track }: MediaRowProps<Item>) {
  const { t } = useTranslation();
  const headingId = useId();
  const { scrollRef, listRef, slots, totalWidth, edges, scrollPage, cardProps } = useMediaRow(
    track.items.length,
    track.format,
  );
  return (
    <section aria-labelledby={headingId} className="group/row relative">
      <RowHeader headingId={headingId} title={title} renderSeeAll={renderSeeAll} />
      <ArrowButton
        direction={-1}
        label={t("mediaRow.previous", { title })}
        atEnd={edges.atStart}
        onScroll={scrollPage}
      />
      <RowTrack
        {...track}
        scrollRef={scrollRef}
        listRef={listRef}
        slots={slots}
        totalWidth={totalWidth}
        cardProps={cardProps}
      />
      <ArrowButton
        direction={1}
        label={t("mediaRow.next", { title })}
        atEnd={edges.atEnd}
        onScroll={scrollPage}
      />
    </section>
  );
}
