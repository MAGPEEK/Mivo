import { useRef, type KeyboardEvent } from "react";

/**
 * Which arrow keys move through a roving group: tabs use the keys of their axis, radio groups
 * accept all four arrows.
 */
export type RovingOrientation = "horizontal" | "vertical" | "both";

const forwardKeys: Record<RovingOrientation, readonly string[]> = {
  horizontal: ["ArrowRight"],
  vertical: ["ArrowDown"],
  both: ["ArrowRight", "ArrowDown"],
};
const backwardKeys: Record<RovingOrientation, readonly string[]> = {
  horizontal: ["ArrowLeft"],
  vertical: ["ArrowUp"],
  both: ["ArrowLeft", "ArrowUp"],
};

/** Index a roving group moves to for an arrow/Home/End key, wrapping at both ends. */
export function rovingTarget(
  key: string,
  current: number,
  count: number,
  orientation: RovingOrientation = "both",
): number | undefined {
  if (forwardKeys[orientation].includes(key)) {
    return (current + 1) % count;
  }
  if (backwardKeys[orientation].includes(key)) {
    return (current - 1 + count) % count;
  }
  if (key === "Home") {
    return 0;
  }
  return key === "End" ? count - 1 : undefined;
}

/** Options of {@link useRovingFocus}. */
export interface RovingFocusOptions {
  readonly count: number;
  /** Index of the item in the Tab order (the selected one, else the first). */
  readonly tabbableIndex: number;
  /** Called with the index that arrow keys moved focus to (selection follows focus). */
  readonly onMove: (index: number) => void;
  readonly orientation?: RovingOrientation;
}

/**
 * Roving tabindex for tabs and radio groups: one item is tabbable, arrow keys move focus and
 * selection. Spread `itemProps(index)` on each item (a button).
 */
export function useRovingFocus({
  count,
  tabbableIndex,
  onMove,
  orientation = "both",
}: RovingFocusOptions) {
  const items = useRef<(HTMLElement | null)[]>([]);
  const onKeyDown = (event: KeyboardEvent<HTMLElement>): void => {
    const current = items.current.findIndex((item) => item === document.activeElement);
    const target = current < 0 ? undefined : rovingTarget(event.key, current, count, orientation);
    if (target === undefined) {
      return;
    }
    event.preventDefault();
    items.current[target]?.focus();
    onMove(target);
  };
  const itemProps = (index: number) => ({
    onKeyDown,
    ref: (element: HTMLElement | null) => {
      items.current[index] = element;
    },
    tabIndex: index === tabbableIndex ? 0 : -1,
  });
  return { itemProps };
}
