import { act, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  controlsHideDelayMilliseconds,
  playerControlsMarker,
  useControlsVisibility,
  useKeyboardFocusInControls,
} from "./useControlsVisibility";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

function renderVisibility(autoHide: boolean) {
  return renderHook(({ hide }: { hide: boolean }) => useControlsVisibility(hide), {
    initialProps: { hide: autoHide },
  });
}

function waitOut(milliseconds = controlsHideDelayMilliseconds) {
  act(() => {
    vi.advanceTimersByTime(milliseconds);
  });
}

describe("useControlsVisibility", () => {
  it("keeps the controls while nothing plays", () => {
    const { result } = renderVisibility(false);

    waitOut(controlsHideDelayMilliseconds * 10);

    expect(result.current).toBe(true);
  });

  it("hides them after three seconds without activity while playing", () => {
    const { result } = renderVisibility(true);

    waitOut(controlsHideDelayMilliseconds - 1);
    expect(result.current).toBe(true);
    waitOut(1);

    expect(result.current).toBe(false);
  });

  it.each(["pointerMove", "pointerDown", "keyDown"] as const)(
    "shows them again on %s and restarts the delay",
    (event) => {
      const { result } = renderVisibility(true);
      waitOut();

      act(() => {
        fireEvent[event](window);
      });
      expect(result.current).toBe(true);
      waitOut(controlsHideDelayMilliseconds - 1);
      expect(result.current).toBe(true);
      waitOut(1);

      expect(result.current).toBe(false);
    },
  );

  it("starts every phase of auto-hiding with visible controls", () => {
    const { result, rerender } = renderVisibility(true);
    waitOut();

    rerender({ hide: false });
    expect(result.current).toBe(true);
    rerender({ hide: true });

    expect(result.current).toBe(true);
    waitOut();
    expect(result.current).toBe(false);
  });

  it("stops listening when unmounted", () => {
    const removeListener = vi.spyOn(window, "removeEventListener");
    const { unmount } = renderVisibility(true);

    unmount();

    expect(removeListener).toHaveBeenCalledWith("pointermove", expect.any(Function));
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("useKeyboardFocusInControls", () => {
  let controls: HTMLElement;
  let button: HTMLButtonElement;
  let focusVisible: boolean;

  beforeEach(() => {
    controls = document.createElement("div");
    for (const [name, value] of Object.entries(playerControlsMarker)) {
      controls.setAttribute(name, value);
    }
    button = document.createElement("button");
    controls.append(button);
    document.body.append(controls);
    focusVisible = false;
    // jsdom has no focus-visible heuristics; a browser decides it from the input method.
    const matches = button.matches.bind(button);
    vi.spyOn(button, "matches").mockImplementation((selectors) =>
      selectors === ":focus-visible" ? focusVisible : matches(selectors),
    );
  });

  afterEach(() => {
    controls.remove();
    vi.restoreAllMocks();
  });

  it("does not pin the controls for a control focused by a mouse click", () => {
    const { result } = renderHook(() => useKeyboardFocusInControls());

    act(() => {
      button.focus();
    });

    expect(result.current).toBe(false);
  });

  it("pins them once a control focused by a click gets keyboard input", () => {
    const { result } = renderHook(() => useKeyboardFocusInControls());
    act(() => {
      button.focus();
    });
    expect(result.current).toBe(false);

    focusVisible = true;
    act(() => {
      fireEvent.keyDown(button, { key: "ArrowRight" });
    });

    expect(result.current).toBe(true);
  });

  it("stays unpinned on keyboard input that leaves the focus without a ring", () => {
    const { result } = renderHook(() => useKeyboardFocusInControls());
    act(() => {
      button.focus();
    });

    act(() => {
      fireEvent.keyDown(button, { key: "Shift" });
    });

    expect(result.current).toBe(false);
  });

  it("does not pin them for keyboard input while focus is outside the controls", () => {
    const { result } = renderHook(() => useKeyboardFocusInControls());
    focusVisible = true;

    act(() => {
      fireEvent.keyDown(document.body, { key: "ArrowRight" });
    });

    expect(result.current).toBe(false);
  });
});
