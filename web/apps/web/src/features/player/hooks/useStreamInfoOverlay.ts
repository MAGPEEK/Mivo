import { useEffect, useRef, useState, type RefObject } from "react";

/** Open state, actions and focus targets of the stream-info overlay. */
export interface StreamInfoOverlay {
  readonly open: boolean;
  readonly toggle: () => void;
  readonly close: () => void;
  /** The control bar's toggle button; focus returns there when the overlay closes. */
  readonly toggleRef: RefObject<HTMLButtonElement | null>;
  /** The overlay's region; tells whether focus was inside when it closes. */
  readonly regionRef: RefObject<HTMLElement | null>;
  /** The overlay's heading (`tabIndex={-1}`); focus moves there when it opens. */
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
}

/**
 * Opens and closes the stream-info overlay with focus management: opening moves focus to its
 * heading, so a screen reader announces the region and the next Tab reaches its close button;
 * closing returns focus to the toggle button when it was inside the overlay, so it is never
 * lost to the document.
 */
export function useStreamInfoOverlay(): StreamInfoOverlay {
  const [open, setOpen] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const regionRef = useRef<HTMLElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const returnFocus = useRef(false);

  useEffect(() => {
    if (open) {
      headingRef.current?.focus();
    } else if (returnFocus.current) {
      returnFocus.current = false;
      toggleRef.current?.focus();
    }
  }, [open]);

  const close = () => {
    returnFocus.current = regionRef.current?.contains(document.activeElement) ?? false;
    setOpen(false);
  };
  return {
    open,
    toggle: () => {
      if (open) {
        close();
      } else {
        setOpen(true);
      }
    },
    close,
    toggleRef,
    regionRef,
    headingRef,
  };
}
