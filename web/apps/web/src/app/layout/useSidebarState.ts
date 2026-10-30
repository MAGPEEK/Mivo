import { productId } from "@mivo/ui-tokens";
import { useCallback, useRef, useState, type RefObject } from "react";

import { browserLocalStorage, readFlag, writeFlag } from "../../shared/lib/persistentFlag";

/** `localStorage` key of the collapsed flag. Derived from the brand id (ADR 0002). */
export const sidebarCollapsedStorageKey = `${productId}.sidebar.collapsed`;

// A first visit on a wide screen shows the full sidebar with its labels.
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
  /**
   * True while the expanded sidebar lies over the page on a narrow screen. Only then is it
   * dismissed by Escape, a press outside, leaving focus or navigation (see
   * `useSidebarOverlay`).
   */
  readonly overlayOpen: boolean;
  /** Closes the overlay; does nothing on wide screens, where expanding is a preference. */
  readonly closeOverlay: () => void;
  /** Attach to the sidebar toggle button. */
  readonly toggleRef: RefObject<HTMLButtonElement | null>;
}

/**
 * Holds whether the sidebar is collapsed. On wide screens the choice is a preference kept in
 * `localStorage` across visits (when the browser blocks storage, until the page is
 * reloaded). On narrow screens the expanded sidebar covers the page, so it is a temporary
 * overlay instead: it starts closed, is never saved, and a wide-screen preference does not
 * open it. Resizing switches between both without losing the preference.
 */
export function useSidebarState(narrowScreen: boolean): SidebarState {
  const [collapsedPreference, setCollapsedPreference] = useState(() =>
    readFlag(browserLocalStorage(), sidebarCollapsedStorageKey, collapsedByDefault),
  );
  const [overlayRequested, setOverlayRequested] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  // Adjusted while rendering rather than in an effect: a window that turns wide forgets the
  // overlay, so turning narrow again starts with it closed.
  if (!narrowScreen && overlayRequested) {
    setOverlayRequested(false);
  }
  const overlayOpen = narrowScreen && overlayRequested;
  const collapsed = narrowScreen ? !overlayOpen : collapsedPreference;

  // Saved in the event handlers rather than an effect, so only a deliberate change persists.
  const changeCollapsed = useCallback(
    (nextCollapsed: boolean) => {
      if (narrowScreen) {
        setOverlayRequested(!nextCollapsed);
        return;
      }
      setCollapsedPreference(nextCollapsed);
      writeFlag(browserLocalStorage(), sidebarCollapsedStorageKey, nextCollapsed);
    },
    [narrowScreen],
  );

  const toggleCollapsed = useCallback(() => {
    changeCollapsed(!collapsed);
  }, [changeCollapsed, collapsed]);

  // The toggle exists in both states, so it can take focus before the re-render.
  const expandAndFocusToggle = useCallback(() => {
    changeCollapsed(false);
    toggleRef.current?.focus();
  }, [changeCollapsed]);

  const closeOverlay = useCallback(() => {
    setOverlayRequested(false);
  }, []);

  return {
    collapsed,
    toggleCollapsed,
    expandAndFocusToggle,
    overlayOpen,
    closeOverlay,
    toggleRef,
  };
}
