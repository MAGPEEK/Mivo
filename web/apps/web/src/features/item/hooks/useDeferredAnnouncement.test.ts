import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useDeferredAnnouncement } from "./useDeferredAnnouncement";

describe("useDeferredAnnouncement", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("is empty on mount and holds the text after the next macrotask", () => {
    const { result } = renderHook(() => useDeferredAnnouncement("Loading item details"));

    expect(result.current).toBe("");

    act(() => {
      vi.runAllTimers();
    });

    expect(result.current).toBe("Loading item details");
  });

  it("follows a changed text once mounted", () => {
    const { result, rerender } = renderHook(({ text }) => useDeferredAnnouncement(text), {
      initialProps: { text: "first" },
    });
    act(() => {
      vi.runAllTimers();
    });

    rerender({ text: "second" });

    expect(result.current).toBe("second");
  });

  it("cancels the pending update when unmounted first", () => {
    const { unmount } = renderHook(() => useDeferredAnnouncement("Loading item details"));

    unmount();

    expect(vi.getTimerCount()).toBe(0);
  });
});
