import { defaultRangeExtractor, useVirtualizer, type Range } from "@tanstack/react-virtual";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type RefObject,
} from "react";

import {
  cardStride,
  cardWidths,
  pageScrollTarget,
  rowGap,
  rowKeyTarget,
  rowScrollEdges,
  shouldVirtualize,
  withActiveIndex,
  type CardFormat,
  type RowScrollEdges,
  type ScrollDirection,
} from "../lib/rowScroll";
import { useMediaQuery } from "./useMediaQuery";

/** What a row card's link takes: the roving tab stop and the keys that move it. */
export interface RowCardProps {
  /** Only one card of the row is in the Tab order. */
  readonly tabIndex: 0 | -1;
  /** Arrow keys move focus to the neighbouring card, Home and End to the ends. */
  readonly onKeyDown: (event: KeyboardEvent<HTMLElement>) => void;
  /** Moves the tab stop to a card focused by click or Tab. */
  readonly onFocus: (event: FocusEvent<HTMLElement>) => void;
}

/** A card the row renders, with its position along the row in CSS pixels. */
export interface RowSlot {
  readonly index: number;
  readonly start: number;
}

// Cards rendered beyond each edge of the view, so a short scroll shows no empty slot.
const overscanCards = 3;
const reducedMotionQuery = "(prefers-reduced-motion: reduce)";
const focusableSelector = "a[href], button:not([disabled]), [tabindex]";
const indexAttribute = "data-index";

function readEdges(element: HTMLElement | null): RowScrollEdges {
  return element === null ? { atStart: true, atEnd: true } : rowScrollEdges(element);
}

/**
 * Tracks whether the row is scrolled to an end. Follows scrolling, resizing of the view and of
 * the list, and a new content width (more items or another card format).
 */
function useRowEdges(
  scrollRef: RefObject<HTMLDivElement | null>,
  listRef: RefObject<HTMLUListElement | null>,
  contentWidth: number,
): RowScrollEdges {
  const [edges, setEdges] = useState<RowScrollEdges>({ atStart: true, atEnd: true });
  useLayoutEffect(() => {
    const element = scrollRef.current;
    const update = () => {
      const next = readEdges(element);
      setEdges((current) =>
        current.atStart === next.atStart && current.atEnd === next.atEnd ? current : next,
      );
    };
    update();
    element?.addEventListener("scroll", update, { passive: true });
    // jsdom and old browsers have no ResizeObserver; the row then updates on scrolling only.
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(update) : undefined;
    for (const observed of [element, listRef.current]) {
      if (observed !== null) {
        observer?.observe(observed);
      }
    }
    return () => {
      element?.removeEventListener("scroll", update);
      observer?.disconnect();
    };
  }, [scrollRef, listRef, contentWidth]);
  return edges;
}

function cardIndexOf(target: EventTarget): number | undefined {
  const slot = target instanceof Element ? target.closest(`[${indexAttribute}]`) : null;
  const value = slot?.getAttribute(indexAttribute);
  return value === null || value === undefined ? undefined : Number(value);
}

/** Roving tab stop over the cards: arrows move focus, Home/End jump, focus moves the stop. */
function useRowFocus(listRef: RefObject<HTMLUListElement | null>, count: number) {
  const [activeIndex, setActiveIndex] = useState(0);
  const pendingFocus = useRef<number | undefined>(undefined);
  // A shorter list (e.g. after a refetch) must not leave the tab stop on a missing card.
  const tabbableIndex = Math.min(activeIndex, Math.max(0, count - 1));
  useEffect(() => {
    if (pendingFocus.current !== tabbableIndex) {
      return;
    }
    pendingFocus.current = undefined;
    const slot = listRef.current?.querySelector(`[${indexAttribute}="${String(tabbableIndex)}"]`);
    slot?.querySelector<HTMLElement>(focusableSelector)?.focus();
  }, [listRef, tabbableIndex]);
  const onKeyDown = useCallback(
    (event: KeyboardEvent<HTMLElement>) => {
      const current = cardIndexOf(event.currentTarget);
      const target = current === undefined ? undefined : rowKeyTarget(event.key, current, count);
      if (target === undefined) {
        return;
      }
      event.preventDefault();
      // The target may not be rendered yet; it mounts as the tab stop, then gets focus.
      pendingFocus.current = target;
      setActiveIndex(target);
    },
    [count],
  );
  const onFocus = useCallback((event: FocusEvent<HTMLElement>) => {
    const index = cardIndexOf(event.currentTarget);
    if (index !== undefined) {
      setActiveIndex(index);
    }
  }, []);
  const cardProps = (index: number): RowCardProps => ({
    tabIndex: index === tabbableIndex ? 0 : -1,
    onKeyDown,
    onFocus,
  });
  return { tabbableIndex, cardProps };
}

/** The cards to render: all of a short row, the visible ones (plus the tab stop) of a long one. */
function useRowSlots(
  scrollRef: RefObject<HTMLDivElement | null>,
  count: number,
  format: CardFormat,
  tabbableIndex: number,
): RowSlot[] {
  const virtual = shouldVirtualize(count);
  const rangeExtractor = useCallback(
    (range: Range) => withActiveIndex(defaultRangeExtractor(range), tabbableIndex),
    [tabbableIndex],
  );
  // The app does not use the React Compiler; the virtualiser re-renders through its own state.
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual (M5-24 notes)
  const virtualizer = useVirtualizer({
    count,
    horizontal: true,
    enabled: virtual,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => cardWidths[format],
    gap: rowGap,
    overscan: overscanCards,
    rangeExtractor,
  });
  // Card sizes are estimated once per index; another format must drop the cached positions
  // before the browser paints, or the row shows one frame with the old offsets.
  useLayoutEffect(() => {
    virtualizer.measure();
  }, [virtualizer, format]);
  const stride = cardStride(format);
  return virtual
    ? virtualizer.getVirtualItems().map(({ index, start }) => ({ index, start }))
    : Array.from({ length: count }, (_, index) => ({ index, start: index * stride }));
}

/**
 * State and handlers of a horizontally scrolling row of cards: which cards to render (only
 * the visible ones in long rows), the roving tab stop and the page-wise arrow buttons.
 */
export function useMediaRow(count: number, format: CardFormat) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const listRef = useRef<HTMLUListElement | null>(null);
  const stride = cardStride(format);
  const totalWidth = Math.max(0, count * stride - rowGap);
  const focus = useRowFocus(listRef, count);
  const edges = useRowEdges(scrollRef, listRef, totalWidth);
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const slots = useRowSlots(scrollRef, count, format, focus.tabbableIndex);
  const scrollPage = (direction: ScrollDirection) => {
    const element = scrollRef.current;
    // The button at a reached end stays focusable (aria-disabled) but does nothing.
    const atEdge = direction < 0 ? edges.atStart : edges.atEnd;
    if (element === null || atEdge) {
      return;
    }
    const left = pageScrollTarget(element, direction, stride);
    element.scrollTo({ left, behavior: reducedMotion ? "auto" : "smooth" });
  };
  return {
    scrollRef,
    listRef,
    slots,
    totalWidth,
    edges,
    scrollPage,
    ...focus,
  };
}
