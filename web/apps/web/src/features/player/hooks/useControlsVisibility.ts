import { useEffect, useState } from "react";

/** Inactivity after which the controls hide while the video plays (task M2-30). */
export const controlsHideDelayMilliseconds = 3_000;

// Any of these shows the controls again and restarts the delay.
const activityEvents = ["keydown", "pointerdown", "pointermove"] as const;

/**
 * Whether the player's controls are visible. With `autoHide` (the video plays and nothing
 * needs the controls), they hide after {@link controlsHideDelayMilliseconds} without pointer
 * or keyboard activity; any activity shows them again. The caller turns `autoHide` off while
 * keyboard focus is on a control ({@link useKeyboardFocusInControls}), so a visible focus ring
 * never fades away.
 */
export function useControlsVisibility(autoHide: boolean): boolean {
  const [idle, setIdle] = useState(false);
  const [previousAutoHide, setPreviousAutoHide] = useState(autoHide);
  // Each phase of auto-hiding starts with visible controls (React's "adjust state when a
  // prop changes" pattern, without an extra effect pass).
  if (autoHide !== previousAutoHide) {
    setPreviousAutoHide(autoHide);
    setIdle(false);
  }

  useEffect(() => {
    if (!autoHide) {
      return;
    }
    const hideLater = () =>
      setTimeout(() => {
        setIdle(true);
      }, controlsHideDelayMilliseconds);
    let timer = hideLater();
    const onActivity = () => {
      clearTimeout(timer);
      setIdle(false);
      timer = hideLater();
    };
    for (const type of activityEvents) {
      window.addEventListener(type, onActivity);
    }
    return () => {
      clearTimeout(timer);
      for (const type of activityEvents) {
        window.removeEventListener(type, onActivity);
      }
    };
  }, [autoHide]);

  return !autoHide || !idle;
}

const playerControlsAttribute = "data-player-controls";

/** Spread onto the top bar and the control bar; keyboard focus inside them keeps them visible. */
export const playerControlsMarker = { [playerControlsAttribute]: "" } as const;

/** Whether `target` is a control the keyboard focused (`:focus-visible`, not a mouse click). */
function isKeyboardFocusedControl(target: EventTarget | null): boolean {
  return (
    target instanceof Element &&
    target.closest(`[${playerControlsAttribute}]`) !== null &&
    target.matches(":focus-visible")
  );
}

/**
 * Whether keyboard focus is inside the top bar or the control bar. A mouse click also focuses
 * a button but shows no focus ring, so it does not count and the controls still hide.
 */
export function useKeyboardFocusInControls(): boolean {
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    const onFocusIn = (event: FocusEvent) => {
      setFocused(isKeyboardFocusedControl(event.target));
    };
    // When focus moves to another element, its focusin follows and decides.
    const onFocusOut = (event: FocusEvent) => {
      if (event.relatedTarget === null) {
        setFocused(false);
      }
    };
    window.addEventListener("focusin", onFocusIn);
    window.addEventListener("focusout", onFocusOut);
    return () => {
      window.removeEventListener("focusin", onFocusIn);
      window.removeEventListener("focusout", onFocusOut);
    };
  }, []);
  return focused;
}
