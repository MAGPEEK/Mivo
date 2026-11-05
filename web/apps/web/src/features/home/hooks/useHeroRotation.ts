import { useCallback, useEffect, useState, useSyncExternalStore, type FocusEvent } from "react";

import { useMediaQuery } from "../../../shared/ui";

/** Time each hero title stays before the next one shows (HO-01). */
export const heroRotationIntervalMs = 8_000;

const reducedMotionQuery = "(prefers-reduced-motion: reduce)";

function subscribeToVisibility(onChange: () => void): () => void {
  document.addEventListener("visibilitychange", onChange);
  return () => {
    document.removeEventListener("visibilitychange", onChange);
  };
}

function documentHidden(): boolean {
  return document.visibilityState === "hidden";
}

/** Handlers that pause the rotation; spread them onto the hero region. */
export interface HeroPauseHandlers {
  readonly onPointerEnter: () => void;
  readonly onPointerLeave: () => void;
  readonly onFocus: () => void;
  readonly onBlur: (event: FocusEvent<HTMLElement>) => void;
}

/** State of the hero rotation. */
export interface HeroRotation {
  /** Index of the title shown, always below the title count (0 without titles). */
  readonly index: number;
  /** Shows a title, e.g. from its dot; the next change follows a full interval later. */
  readonly select: (index: number) => void;
  /** False while paused or with reduced motion, so screen readers can hear changes. */
  readonly rotating: boolean;
  /**
   * Whether the rotation can run at all: more than one title and no reduced motion. Only then
   * is the stop control offered.
   */
  readonly canRotate: boolean;
  /** True after the user stopped the rotation; it stays stopped until started again. */
  readonly stopped: boolean;
  /** Stops a running rotation or starts a stopped one (WCAG 2.2.2). */
  readonly toggleStopped: () => void;
  readonly pauseHandlers: HeroPauseHandlers;
}

/**
 * Rotates through `count` hero titles every {@link heroRotationIntervalMs} (HO-01). It pauses
 * while the pointer is over the hero, while focus is inside it and while the tab is hidden, and
 * stays stopped after `toggleStopped` until started again. With `prefers-reduced-motion` it
 * never rotates: the first title stays and the dots change it.
 */
export function useHeroRotation(count: number): HeroRotation {
  const [selected, setSelected] = useState(0);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [stopped, setStopped] = useState(false);
  const hidden = useSyncExternalStore(subscribeToVisibility, documentHidden);
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  // A shorter list after a refresh must not point past its end.
  const index = count > 0 ? selected % count : 0;
  const canRotate = count > 1 && !reducedMotion;
  const rotating = canRotate && !stopped && !hovered && !focused && !hidden;

  // Restarted on every change of the title, so a manual choice also gets a full interval.
  useEffect(() => {
    if (!rotating) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      setSelected((index + 1) % count);
    }, heroRotationIntervalMs);
    return () => {
      window.clearTimeout(timer);
    };
  }, [rotating, index, count]);

  const select = useCallback((next: number) => {
    setSelected(next);
  }, []);

  const toggleStopped = useCallback(() => {
    setStopped((current) => !current);
  }, []);

  const pauseHandlers: HeroPauseHandlers = {
    onPointerEnter: () => {
      setHovered(true);
    },
    onPointerLeave: () => {
      setHovered(false);
    },
    onFocus: () => {
      setFocused(true);
    },
    // Moving focus between the hero's own controls keeps it paused.
    onBlur: (event) => {
      if (!(
        event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)
      )) {
        setFocused(false);
      }
    },
  };

  return { index, select, rotating, canRotate, stopped, toggleStopped, pauseHandlers };
}
