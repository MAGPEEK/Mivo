import { useEffect, useRef, type RefObject } from "react";

import { isSearchShortcut } from "../../shared/lib/searchShortcut";

/**
 * Makes Ctrl+K / Cmd+K focus the global search field from anywhere on the page. Returns the
 * ref to attach to that field. The browser default (e.g. focusing its own search bar) is
 * prevented only while the field exists, and a shortcut another handler already claimed is
 * left alone.
 */
export function useSearchShortcut(): RefObject<HTMLInputElement | null> {
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      const searchInput = searchInputRef.current;
      if (searchInput === null || event.defaultPrevented || !isSearchShortcut(event)) {
        return;
      }
      event.preventDefault();
      searchInput.focus();
      // Selecting lets the user type a new query right away instead of appending to the old one.
      searchInput.select();
    };
    window.addEventListener("keydown", focusSearch);
    return () => {
      window.removeEventListener("keydown", focusSearch);
    };
  }, []);

  return searchInputRef;
}
