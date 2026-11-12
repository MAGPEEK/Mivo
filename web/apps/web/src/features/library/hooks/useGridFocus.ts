import {
  useCallback,
  useEffect,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { focusTarget, type GridKey } from "../lib/letterOffsets";

/** Attribute of a grid cell that holds its list position. */
export const cellIndexAttribute = "data-index";

const gridKeys: ReadonlySet<string> = new Set<GridKey>([
  "ArrowLeft",
  "ArrowRight",
  "ArrowUp",
  "ArrowDown",
  "PageUp",
  "PageDown",
]);

/** What the key handling needs to know about the layout. */
export interface GridMetrics {
  readonly columns: number;
  readonly visibleRows: number;
  /** Items that have a card; skeletons cannot take focus. */
  readonly itemCount: number;
}

/** Handlers for the list element. */
export interface GridListProps {
  readonly onKeyDown: (event: KeyboardEvent<HTMLElement>) => void;
  readonly onFocus: (event: FocusEvent<HTMLElement>) => void;
  readonly onBlur: (event: FocusEvent<HTMLElement>) => void;
}

/** The focus state of a virtual grid and the handlers that keep it. */
export interface GridFocus {
  /** Position of the card that holds the focus; its row stays mounted while it does. */
  readonly pinnedIndex: number | undefined;
  /** Moves the focus to the card at `index` as soon as it is in the DOM. */
  /** The focus move that waits for its card to mount; see {@link useFocusRequest}. */
  readonly request: FocusRequest | undefined;
  /** Marks the pending move as done. */
  readonly clearRequest: () => void;
  readonly requestFocus: (index: number, options?: { readonly scroll: boolean }) => void;
  /** The handlers of the list element, for the current layout. */
  readonly listProps: (metrics: GridMetrics) => GridListProps;
}

/** A pending move of the focus to the card at `index`. */
export interface FocusRequest {
  readonly index: number;
  /** The browser scrolls a focused element into view; a jump has scrolled already. */
  readonly scroll: boolean;
}

function indexOfCell(target: EventTarget): number | undefined {
  const cell = target instanceof Element ? target.closest(`[${cellIndexAttribute}]`) : null;
  const index = Number(cell?.getAttribute(cellIndexAttribute));
  return cell === null || Number.isNaN(index) ? undefined : index;
}

/**
 * Arrow keys and Page Up/Down move the focus from card to card (LB-30). The card that holds the
 * focus keeps its row mounted (`pinnedIndex`), so scrolling never drops the focus to the
 * document; a target row that is not mounted yet is focused as soon as it is.
 */
export function useGridFocus(): GridFocus {
  const [pinnedIndex, setPinnedIndex] = useState<number>();
  const [request, setRequest] = useState<FocusRequest>();
  const requestFocus = useCallback((index: number, options = { scroll: false }) => {
    setPinnedIndex(index);
    setRequest({ index, scroll: options.scroll });
  }, []);
  const listProps = (metrics: GridMetrics): GridListProps => ({
    onKeyDown: (event) => {
      const target = keyTarget(event, metrics);
      if (target !== undefined) {
        event.preventDefault();
        requestFocus(target, { scroll: true });
      }
    },
    onFocus: (event) => {
      setPinnedIndex(indexOfCell(event.target));
    },
    onBlur: (event) => {
      if (!event.currentTarget.contains(event.relatedTarget)) {
        setPinnedIndex(undefined);
      }
    },
  });
  const clearRequest = useCallback(() => {
    setRequest(undefined);
  }, []);
  return { pinnedIndex, request, clearRequest, requestFocus, listProps };
}

// The position a key press moves to; modified keys and other keys are the browser's.
function keyTarget(event: KeyboardEvent<HTMLElement>, metrics: GridMetrics): number | undefined {
  const index = indexOfCell(event.target);
  const modified = event.altKey || event.ctrlKey || event.metaKey;
  if (index === undefined || modified || !gridKeys.has(event.key)) {
    return undefined;
  }
  return focusTarget({ ...metrics, key: event.key as GridKey, index });
}

/** Options of {@link useFocusRequest}. */
export interface FocusRequestOptions {
  readonly listRef: RefObject<HTMLElement | null>;
  readonly request: FocusRequest | undefined;
  readonly clearRequest: () => void;
  /** Changes whenever other cells are in the DOM, so a card that just mounted is found. */
  readonly mountedKey: string;
}

/** Focuses the card of a pending move as soon as it is in the DOM. */
export function useFocusRequest({
  listRef,
  request,
  clearRequest,
  mountedKey,
}: FocusRequestOptions): void {
  useEffect(() => {
    if (request === undefined) {
      return;
    }
    const link = listRef.current?.querySelector<HTMLElement>(
      `[${cellIndexAttribute}="${String(request.index)}"] a`,
    );
    if (link !== null && link !== undefined) {
      link.focus({ preventScroll: !request.scroll });
      clearRequest();
    }
  }, [listRef, request, clearRequest, mountedKey]);
}
