import { useEffect, useRef } from "react";

import { browserSessionStorage, readScroll, writeScroll } from "../lib/scrollMemory";

/** Options of {@link useScrollMemory}. */
export interface ScrollMemoryOptions {
  /** Identifies the listing; `undefined` turns the memory off. */
  readonly scrollKey: string | undefined;
  /** Whether the list has its full height, so the remembered position can be reached. */
  readonly ready: boolean;
  /** Where the position is kept; defaults to the tab's `sessionStorage`. */
  readonly storage?: Storage | undefined;
}

/**
 * Brings the page back to where the user left a listing (Back from an item page) and starts a
 * different listing at the top (LB-30). The position is written while scrolling, because the
 * router has scrolled to the top by the time the grid unmounts.
 */
export function useScrollMemory({
  scrollKey,
  ready,
  storage = browserSessionStorage(),
}: ScrollMemoryOptions): void {
  const restored = useRef(false);
  useEffect(() => {
    if (scrollKey === undefined || !ready || restored.current) {
      return;
    }
    restored.current = true;
    window.scrollTo({ top: readScroll(storage, scrollKey) ?? 0 });
  }, [scrollKey, ready, storage]);
  useEffect(() => {
    if (scrollKey === undefined) {
      return undefined;
    }
    let frame = 0;
    const save = () => {
      frame = 0;
      // Before the restore the page is still at the top and would overwrite the memory.
      if (restored.current) {
        writeScroll(storage, { key: scrollKey, top: window.scrollY });
      }
    };
    const onScroll = () => {
      frame ||= window.requestAnimationFrame(save);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.cancelAnimationFrame(frame);
    };
  }, [scrollKey, storage]);
}
