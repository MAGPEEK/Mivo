import { useEffect, useRef, type RefObject } from "react";

import { movePadFocus, padKeyForKeyboard, type PadKey } from "../lib/pinPad";

/** Attribute that numbers the pad buttons in {@link padKeys} order. */
export const padIndexAttribute = "data-pad-index";

function focusedPadIndex(container: HTMLElement): number | undefined {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement) || !container.contains(active)) {
    return undefined;
  }
  const value = active.getAttribute(padIndexAttribute);
  return value === null ? undefined : Number(value);
}

function focusPadButton(container: HTMLElement, index: number): void {
  container.querySelector<HTMLElement>(`[${padIndexAttribute}="${String(index)}"]`)?.focus();
}

function isModified(event: KeyboardEvent): boolean {
  return event.ctrlKey || event.metaKey || event.altKey;
}

/**
 * Keyboard and TV remote on the PIN panel: digit keys and Backspace type anywhere inside it,
 * arrow keys move between the pad buttons, and Enter presses the focused button natively.
 * A native listener, because the panel is a plain region rather than an interactive widget.
 */
export function usePinKeyboard(
  containerRef: RefObject<HTMLElement | null>,
  onKey: (key: PadKey) => void,
): void {
  // The newest handler without re-attaching the listener on every render.
  const onKeyRef = useRef(onKey);
  useEffect(() => {
    onKeyRef.current = onKey;
  });
  useEffect(() => {
    const container = containerRef.current;
    if (container === null) {
      return undefined;
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (isModified(event)) {
        return;
      }
      const padKey = padKeyForKeyboard(event.key);
      if (padKey !== undefined) {
        event.preventDefault();
        onKeyRef.current(padKey);
        return;
      }
      const index = focusedPadIndex(container);
      const target = index === undefined ? undefined : movePadFocus(index, event.key);
      if (target !== undefined) {
        event.preventDefault();
        focusPadButton(container, target);
      }
    };
    container.addEventListener("keydown", onKeyDown);
    return () => {
      container.removeEventListener("keydown", onKeyDown);
    };
  }, [containerRef]);
}
