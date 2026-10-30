import { useCallback, useEffect, useMemo, useState } from "react";

const dismissKey = "Escape";
const captureOptions: AddEventListenerOptions = { capture: true };

/** Event handlers for the element that shows the tooltip, e.g. a button. */
export interface TooltipTriggerHandlers {
  readonly onMouseEnter: () => void;
  readonly onMouseLeave: () => void;
  readonly onFocus: () => void;
  readonly onBlur: () => void;
}

/** Visibility of a tooltip and the handlers that drive it. */
export interface DismissibleTooltip {
  readonly open: boolean;
  readonly triggerHandlers: TooltipTriggerHandlers;
}

/**
 * A tooltip that follows WCAG 1.4.13 (content on hover or focus): it shows while its trigger
 * is hovered or focused, stays as long as either lasts and closes on Escape without moving
 * focus. Hovering or focusing the trigger again shows it again.
 *
 * The tooltip must sit inside the trigger, so hovering the tooltip counts as hovering the
 * trigger, and leave no gap between them that the pointer has to cross.
 */
export function useDismissibleTooltip(): DismissibleTooltip {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const open = (hovered || focused) && !dismissed;

  // Listens on the document: a hovered tooltip must close on Escape while focus is elsewhere.
  // The capture phase runs first, so a handler that stops Escape from bubbling cannot keep
  // the tooltip open.
  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const dismissOnEscape = (event: KeyboardEvent) => {
      if (event.key === dismissKey) {
        setDismissed(true);
      }
    };
    document.addEventListener("keydown", dismissOnEscape, captureOptions);
    return () => {
      document.removeEventListener("keydown", dismissOnEscape, captureOptions);
    };
  }, [open]);

  const show = useCallback((setActive: (active: boolean) => void) => {
    setActive(true);
    setDismissed(false);
  }, []);

  const triggerHandlers = useMemo<TooltipTriggerHandlers>(
    () => ({
      onMouseEnter: () => {
        show(setHovered);
      },
      onMouseLeave: () => {
        setHovered(false);
      },
      onFocus: () => {
        show(setFocused);
      },
      onBlur: () => {
        setFocused(false);
      },
    }),
    [show],
  );

  return { open, triggerHandlers };
}
