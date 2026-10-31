import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useScrubber } from "./useScrubber";

const dragEndEvents = new Set(["pointerup", "pointercancel"]);

/** Follows the drag-end listeners on the window: added minus removed, per event type. */
function trackDragEndListeners(): () => Map<string, Set<EventListenerOrEventListenerObject>> {
  const active = new Map<string, Set<EventListenerOrEventListenerObject>>();
  const add = window.addEventListener.bind(window);
  const remove = window.removeEventListener.bind(window);
  vi.spyOn(window, "addEventListener").mockImplementation((type, listener, options) => {
    if (dragEndEvents.has(type)) {
      active.set(type, (active.get(type) ?? new Set()).add(listener));
    }
    add(type, listener, options);
  });
  vi.spyOn(window, "removeEventListener").mockImplementation((type, listener, options) => {
    active.get(type)?.delete(listener);
    remove(type, listener, options);
  });
  return () => new Map([...active].filter(([, listeners]) => listeners.size > 0));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useScrubber", () => {
  it("keeps one pair of drag-end listeners when a second drag starts before the first ended", () => {
    const activeListeners = trackDragEndListeners();
    const seek = vi.fn<(position: number) => void>();
    const { result, unmount } = renderHook(() => useScrubber(30, seek));

    act(() => {
      result.current.onPointerDown();
    });
    act(() => {
      result.current.onPointerDown();
    });
    expect([...(activeListeners().get("pointerup") ?? [])]).toHaveLength(1);
    expect([...(activeListeners().get("pointercancel") ?? [])]).toHaveLength(1);
    act(() => {
      result.current.onValueChange(70);
    });
    act(() => {
      window.dispatchEvent(new Event("pointerup"));
    });

    expect(seek.mock.calls).toEqual([[70]]);
    expect(activeListeners().size).toBe(0);
    unmount();
    expect(activeListeners().size).toBe(0);
  });

  it("removes the listeners of a drag that is still running when it unmounts", () => {
    const activeListeners = trackDragEndListeners();
    const { result, unmount } = renderHook(() => useScrubber(30, vi.fn()));
    act(() => {
      result.current.onPointerDown();
    });
    act(() => {
      result.current.onPointerDown();
    });

    unmount();

    expect(activeListeners().size).toBe(0);
  });
});
