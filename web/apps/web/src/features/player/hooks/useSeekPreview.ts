import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";

import { scrubberStepSeconds } from "../lib/playerKeys";
import { positionAtPointer } from "../lib/seekPreview";
import { trackPercent } from "../lib/scrubberTrack";
import { useTrickplayTile, type PreviewTile, type TrickplayServices } from "./useTrickplay";

/** How long the preview stays after the last arrow key press, in milliseconds. */
export const keyboardPreviewMilliseconds = 1500;

/** Input of {@link useSeekPreview}. */
export interface SeekPreviewInput {
  /** File whose trickplay is shown; `undefined` before the first session. */
  readonly fileId: string | undefined;
  readonly duration: number | undefined;
  /** Where the thumb is. */
  readonly value: number;
  /** Whether the thumb is being dragged (mouse, pen or a touch). */
  readonly dragging: boolean;
  readonly services?: TrickplayServices | undefined;
}

/** What the scrubber needs to show the preview bubble. */
export interface SeekPreview {
  /** The position the bubble shows; `undefined` hides it. */
  readonly seconds: number | undefined;
  /** Where the bubble sits on the track, in percent of the duration. */
  readonly percent: number;
  /** The tile to draw; `undefined` shows the time only. */
  readonly tile: PreviewTile | undefined;
  readonly onPointerMove: (event: PointerEvent<HTMLElement>) => void;
  readonly onPointerLeave: () => void;
  readonly onKeyDown: (event: KeyboardEvent) => void;
  readonly onBlur: () => void;
}

interface KeyboardPreview {
  readonly seconds: number | undefined;
  readonly onKeyDown: (event: KeyboardEvent) => void;
  readonly clear: () => void;
}

/** The position an arrow key moves to, kept for a moment so the same preview shows. */
function useKeyboardPreview(value: number, duration: number | undefined): KeyboardPreview {
  const [seconds, setSeconds] = useState<number | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const clear = () => {
    clearTimeout(timer.current);
    setSeconds(undefined);
  };
  useEffect(
    () => () => {
      clearTimeout(timer.current);
    },
    [],
  );
  const onKeyDown = (event: KeyboardEvent) => {
    const step = scrubberStepSeconds(event.key);
    if (step === undefined || duration === undefined) {
      return;
    }
    setSeconds(Math.min(duration, Math.max(0, (seconds ?? value) + step)));
    clearTimeout(timer.current);
    timer.current = setTimeout(clear, keyboardPreviewMilliseconds);
  };
  return { seconds, onKeyDown, clear };
}

/**
 * The seek-bar preview (PL-05): which position the bubble shows (hover, drag or arrow keys, in
 * that priority: a drag first) and the trickplay tile of it. Touch has no hover; a drag with
 * a finger shows the bubble like a mouse drag does.
 */
export function useSeekPreview(input: SeekPreviewInput): SeekPreview {
  const { fileId, duration, value, dragging, services } = input;
  const [hovered, setHovered] = useState<number | undefined>(undefined);
  const keyboard = useKeyboardPreview(value, duration);
  const picked = dragging ? value : (hovered ?? keyboard.seconds);
  const seconds = duration === undefined ? undefined : picked;
  const tile = useTrickplayTile(fileId, seconds, services);
  return {
    seconds,
    percent: trackPercent(seconds ?? 0, duration),
    tile,
    onPointerMove: (event) => {
      if (event.pointerType === "touch") {
        return;
      }
      setHovered(
        positionAtPointer(event.clientX, event.currentTarget.getBoundingClientRect(), duration),
      );
    },
    onPointerLeave: () => {
      setHovered(undefined);
    },
    onKeyDown: keyboard.onKeyDown,
    onBlur: keyboard.clear,
  };
}
