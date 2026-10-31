import { act, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { controlsHideDelayMilliseconds, useControlsVisibility } from "./useControlsVisibility";

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
