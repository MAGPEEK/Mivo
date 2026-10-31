import { useEffect, useRef, useState, type KeyboardEvent } from "react";

import { scrubberStepSeconds } from "../lib/playerKeys";

/** Value and event handlers of the scrubber slider. */
export interface Scrubber {
  /** Where the thumb is: the dragged position while dragging, else the playback position. */
  readonly value: number;
  readonly onValueChange: (value: number) => void;
  readonly onPointerDown: () => void;
  readonly onKeyDown: (event: KeyboardEvent) => void;
}

/** Window listeners for the end of a drag. */
interface DragEndListeners {
  /** Listens for `pointerup` (release) and `pointercancel` (cancel) on the window. */
  readonly listen: (onRelease: () => void, onCancel: () => void) => void;
  readonly stopListening: () => void;
}

/** Listeners for the end of a drag that are removed again, also when the page unmounts. */
function useDragEndListeners(): DragEndListeners {
  const remove = useRef<(() => void) | undefined>(undefined);
  useEffect(
    () => () => {
      remove.current?.();
    },
    [],
  );
  return {
    listen: (onRelease, onCancel) => {
      // A second pointerdown can arrive before the first drag ended (e.g. another touch).
      remove.current?.();
      window.addEventListener("pointerup", onRelease);
      window.addEventListener("pointercancel", onCancel);
      remove.current = () => {
        window.removeEventListener("pointerup", onRelease);
        window.removeEventListener("pointercancel", onCancel);
        remove.current = undefined;
      };
    },
    stopListening: () => {
      remove.current?.();
    },
  };
}

/**
 * Seeks from the scrubber. A drag only moves the thumb and seeks once on release, so an HLS
 * stream restarts once instead of for every pixel; a cancelled drag (`pointercancel`, e.g. the
 * browser took over the touch) does not seek. Clicks on the track and keyboard changes seek
 * right away; the arrow keys move ±10 s like the player's shortcuts.
 */
export function useScrubber(position: number, seek: (position: number) => void): Scrubber {
  const [dragValue, setDragValue] = useState<number | undefined>(undefined);
  const dragging = useRef(false);
  const latestDragValue = useRef<number | undefined>(undefined);
  const { listen, stopListening } = useDragEndListeners();

  const endDrag = (commit: boolean) => {
    stopListening();
    dragging.current = false;
    const target = latestDragValue.current;
    latestDragValue.current = undefined;
    setDragValue(undefined);
    if (commit && target !== undefined) {
      seek(target);
    }
  };
  return {
    value: dragValue ?? position,
    onValueChange: (value) => {
      if (dragging.current) {
        latestDragValue.current = value;
        setDragValue(value);
      } else {
        seek(value);
      }
    },
    // The release may happen outside the slider, so the window listens for it.
    onPointerDown: () => {
      dragging.current = true;
      listen(
        () => {
          endDrag(true);
        },
        () => {
          endDrag(false);
        },
      );
    },
    onKeyDown: (event) => {
      const step = scrubberStepSeconds(event.key);
      if (step !== undefined) {
        event.preventDefault();
        seek(position + step);
      }
    },
  };
}
