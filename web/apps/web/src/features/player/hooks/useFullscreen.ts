import { useSyncExternalStore } from "react";

/** Full-screen state of the player and the actions on it. */
export interface Fullscreen {
  /** Whether the browser lets the page go full screen (not on iPhone Safari, for example). */
  readonly supported: boolean;
  readonly active: boolean;
  readonly toggle: () => void;
  /** Leaves full screen; does nothing when the player is not full screen. */
  readonly exit: () => void;
}

function subscribe(listener: () => void): () => void {
  document.addEventListener("fullscreenchange", listener);
  return () => {
    document.removeEventListener("fullscreenchange", listener);
  };
}

/**
 * Shows `element` (the player with its controls, so they stay usable) full screen. Requests
 * the browser refuses, e.g. without a user gesture, change nothing.
 */
export function useFullscreen(element: HTMLElement | null): Fullscreen {
  const active = useSyncExternalStore(
    subscribe,
    () => element !== null && document.fullscreenElement === element,
  );
  const exit = () => {
    if (active) {
      document.exitFullscreen().catch(() => undefined);
    }
  };
  return {
    supported: element !== null && document.fullscreenEnabled,
    active,
    toggle: () => {
      if (active) {
        exit();
      } else {
        element?.requestFullscreen().catch(() => undefined);
      }
    },
    exit,
  };
}
