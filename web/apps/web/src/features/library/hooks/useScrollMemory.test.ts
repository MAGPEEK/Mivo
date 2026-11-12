import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readScroll, writeScroll } from "../lib/scrollMemory";
import { useScrollMemory } from "./useScrollMemory";

const storage = window.sessionStorage;

function setScrollY(top: number): void {
  Object.defineProperty(window, "scrollY", { value: top, configurable: true, writable: true });
}

describe("useScrollMemory", () => {
  beforeEach(() => {
    storage.clear();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    setScrollY(0);
  });

  it("restores the position of the same listing once the list is ready", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    writeScroll(storage, { key: "lib|q", top: 2400 });

    const { rerender } = renderHook(
      (ready: boolean) => {
        useScrollMemory({ scrollKey: "lib|q", ready, storage });
      },
      { initialProps: false },
    );
    expect(scrollTo).not.toHaveBeenCalled();
    rerender(true);

    expect(scrollTo).toHaveBeenCalledWith({ top: 2400 });
    rerender(true);
    expect(scrollTo).toHaveBeenCalledTimes(1);
  });

  it("starts another listing at the top", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    writeScroll(storage, { key: "lib|q", top: 2400 });

    renderHook(() => {
      useScrollMemory({ scrollKey: "lib|other", ready: true, storage });
    });

    expect(scrollTo).toHaveBeenCalledWith({ top: 0 });
  });

  it("remembers the position while the page scrolls, but not before the restore", () => {
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    const { rerender } = renderHook(
      (ready: boolean) => {
        useScrollMemory({ scrollKey: "lib|q", ready, storage });
      },
      { initialProps: false },
    );

    setScrollY(50);
    window.dispatchEvent(new Event("scroll"));
    vi.advanceTimersByTime(50);
    expect(readScroll(storage, "lib|q")).toBeUndefined();

    rerender(true);
    setScrollY(900);
    window.dispatchEvent(new Event("scroll"));
    vi.advanceTimersByTime(50);

    expect(readScroll(storage, "lib|q")).toBe(900);
  });

  it("keeps nothing without a key", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);

    renderHook(() => {
      useScrollMemory({ scrollKey: undefined, ready: true, storage });
    });
    setScrollY(300);
    window.dispatchEvent(new Event("scroll"));
    vi.advanceTimersByTime(50);

    expect(scrollTo).not.toHaveBeenCalled();
    expect(storage.length).toBe(0);
  });
});
