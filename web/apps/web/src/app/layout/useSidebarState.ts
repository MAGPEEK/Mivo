import { productId } from "@mivo/ui-tokens";
import { useCallback, useRef, useState, type RefObject } from "react";

import { browserLocalStorage, readFlag, writeFlag } from "../../shared/lib/persistentFlag";

/** `localStorage` key of the collapsed flag. Derived from the brand id (ADR 0002). */
export const sidebarCollapsedStorageKey = `${productId}.sidebar.collapsed`;

// A first visit shows the full sidebar with its labels.
const collapsedByDefault = false;

/** Collapsed state of the sidebar and the actions that change it. */
export interface SidebarState {
  readonly collapsed: boolean;
  readonly toggleCollapsed: () => void;
  /**
   * Expands the sidebar and moves focus to its toggle. For controls that only exist while
   * collapsed: they vanish on expanding, and keyboard focus must not fall back to the page.
   */
  readonly expandAndFocusToggle: () => void;
  /** Attach to the sidebar toggle button. */
  readonly toggleRef: RefObject<HTMLButtonElement | null>;
}

/**
 * Holds whether the sidebar is collapsed and remembers the choice in `localStorage` across
 * visits. When the browser blocks storage, the choice lasts until the page is reloaded.
 */
export function useSidebarState(): SidebarState {
  const [collapsed, setCollapsed] = useState(() =>
    readFlag(browserLocalStorage(), sidebarCollapsedStorageKey, collapsedByDefault),
  );
  const toggleRef = useRef<HTMLButtonElement>(null);

  // Saved in the event handlers rather than an effect, so only a deliberate change persists.
  const changeCollapsed = useCallback((nextCollapsed: boolean) => {
    setCollapsed(nextCollapsed);
    writeFlag(browserLocalStorage(), sidebarCollapsedStorageKey, nextCollapsed);
  }, []);

  const toggleCollapsed = useCallback(() => {
    changeCollapsed(!collapsed);
  }, [changeCollapsed, collapsed]);

  // The toggle exists in both states, so it can take focus before the re-render.
  const expandAndFocusToggle = useCallback(() => {
    changeCollapsed(false);
    toggleRef.current?.focus();
  }, [changeCollapsed]);

  return { collapsed, toggleCollapsed, expandAndFocusToggle, toggleRef };
}
