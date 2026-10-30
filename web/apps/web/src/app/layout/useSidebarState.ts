import { productId } from "@mivo/ui-tokens";
import { useCallback, useState } from "react";

import { browserLocalStorage, readFlag, writeFlag } from "../../shared/lib/persistentFlag";

/** `localStorage` key of the collapsed flag. Derived from the brand id (ADR 0002). */
export const sidebarCollapsedStorageKey = `${productId}.sidebar.collapsed`;

// A first visit shows the full sidebar with its labels.
const collapsedByDefault = false;

/** Collapsed state of the sidebar and the action that flips it. */
export interface SidebarState {
  readonly collapsed: boolean;
  readonly toggleCollapsed: () => void;
}

/**
 * Holds whether the sidebar is collapsed and remembers the choice across visits.
 * @param storage where the choice is saved; defaults to `localStorage`, `undefined` keeps it
 *   in memory only.
 */
export function useSidebarState(
  storage: Storage | undefined = browserLocalStorage(),
): SidebarState {
  const [collapsed, setCollapsed] = useState(() =>
    readFlag(storage, sidebarCollapsedStorageKey, collapsedByDefault),
  );

  // Saved in the event handler rather than an effect, so only a deliberate toggle persists.
  const toggleCollapsed = useCallback(() => {
    const nextCollapsed = !collapsed;
    setCollapsed(nextCollapsed);
    writeFlag(storage, sidebarCollapsedStorageKey, nextCollapsed);
  }, [collapsed, storage]);

  return { collapsed, toggleCollapsed };
}
