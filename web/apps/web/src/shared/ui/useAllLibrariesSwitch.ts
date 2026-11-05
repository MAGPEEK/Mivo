import { useEffect, useRef, useState, type RefObject } from "react";

/** Options of {@link useAllLibrariesSwitch}. */
export interface AllLibrariesSwitchOptions {
  readonly collapsed: boolean;
  readonly onExpand: () => void;
  readonly doneRef: RefObject<HTMLButtonElement | null>;
  readonly showAllRef: RefObject<HTMLButtonElement | null>;
}

/** State and actions of {@link useAllLibrariesSwitch}. */
export interface AllLibrariesSwitch {
  readonly showAll: boolean;
  readonly show: () => void;
  readonly hide: () => void;
}

/**
 * Flips a libraries view between the pinned list and "All libraries" (sidebar and phone
 * sheet). The pressed control vanishes, so focus moves to its counterpart. Collapsing returns
 * to the pinned list, the only one with an icon-only form.
 */
export function useAllLibrariesSwitch(options: AllLibrariesSwitchOptions): AllLibrariesSwitch {
  const { collapsed, onExpand, doneRef, showAllRef } = options;
  const [requested, setRequested] = useState(false);
  const pendingFocus = useRef<RefObject<HTMLButtonElement | null> | null>(null);
  // Adjusted while rendering rather than in an effect, like useSidebarState's overlay.
  if (collapsed && requested) {
    setRequested(false);
  }
  useEffect(() => {
    const target = pendingFocus.current;
    pendingFocus.current = null;
    target?.current?.focus();
  });
  return {
    showAll: requested && !collapsed,
    show: () => {
      if (collapsed) {
        onExpand();
      }
      setRequested(true);
      pendingFocus.current = doneRef;
    },
    hide: () => {
      setRequested(false);
      pendingFocus.current = showAllRef;
    },
  };
}
