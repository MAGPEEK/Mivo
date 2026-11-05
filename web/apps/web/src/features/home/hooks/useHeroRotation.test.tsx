import { act, renderHook } from "@testing-library/react";
import type { FocusEvent } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { heroRotationIntervalMs, useHeroRotation } from "./useHeroRotation";

const titleCount = 3;

function advance(milliseconds: number) {
  act(() => {
    vi.advanceTimersByTime(milliseconds);
  });
}

function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value: state });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

function stubReducedMotion(matches: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: matches && query.includes("prefers-reduced-motion"),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

// A blur whose focus goes to `next`, or leaves the page without one.
function blurTo(region: HTMLElement, next: Element | null): FocusEvent<HTMLElement> {
  return { currentTarget: region, relatedTarget: next } as unknown as FocusEvent<HTMLElement>;
}

describe("useHeroRotation", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    setVisibility("visible");
  });

  it("moves to the next title every interval and wraps around", () => {
    const { result } = renderHook(() => useHeroRotation(titleCount));
    expect(result.current.index).toBe(0);
    expect(result.current.rotating).toBe(true);

    advance(heroRotationIntervalMs - 1);
    expect(result.current.index).toBe(0);
    advance(1);
    expect(result.current.index).toBe(1);
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(2);
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(0);
  });

  it("gives a selected title a full interval", () => {
    const { result } = renderHook(() => useHeroRotation(titleCount));
    advance(heroRotationIntervalMs / 2);

    act(() => {
      result.current.select(2);
    });
    advance(heroRotationIntervalMs - 1);
    expect(result.current.index).toBe(2);
    advance(1);
    expect(result.current.index).toBe(0);
  });

  it("pauses while hovered", () => {
    const { result } = renderHook(() => useHeroRotation(titleCount));

    act(() => {
      result.current.pauseHandlers.onPointerEnter();
    });
    advance(heroRotationIntervalMs * 2);
    expect(result.current.index).toBe(0);
    expect(result.current.rotating).toBe(false);

    act(() => {
      result.current.pauseHandlers.onPointerLeave();
    });
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(1);
  });

  it("pauses while focus is inside, also when it moves between its controls", () => {
    const region = document.createElement("section");
    const inner = document.createElement("button");
    region.append(inner);
    const { result } = renderHook(() => useHeroRotation(titleCount));

    act(() => {
      result.current.pauseHandlers.onFocus();
      result.current.pauseHandlers.onBlur(blurTo(region, inner));
    });
    advance(heroRotationIntervalMs * 2);
    expect(result.current.index).toBe(0);

    act(() => {
      result.current.pauseHandlers.onBlur(blurTo(region, null));
    });
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(1);
  });

  it("stays stopped until started again", () => {
    const { result } = renderHook(() => useHeroRotation(titleCount));
    expect(result.current.canRotate).toBe(true);
    expect(result.current.stopped).toBe(false);

    act(() => {
      result.current.toggleStopped();
    });
    advance(heroRotationIntervalMs * 2);
    expect(result.current.index).toBe(0);
    expect(result.current.stopped).toBe(true);
    expect(result.current.rotating).toBe(false);

    // Leaving the hero does not resume a stopped rotation.
    act(() => {
      result.current.pauseHandlers.onPointerEnter();
      result.current.pauseHandlers.onPointerLeave();
    });
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(0);

    act(() => {
      result.current.toggleStopped();
    });
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(1);
  });

  it("pauses while the tab is hidden", () => {
    const { result } = renderHook(() => useHeroRotation(titleCount));

    setVisibility("hidden");
    advance(heroRotationIntervalMs * 2);
    expect(result.current.index).toBe(0);

    setVisibility("visible");
    advance(heroRotationIntervalMs);
    expect(result.current.index).toBe(1);
  });

  it("never rotates with reduced motion, but the dots still select", () => {
    stubReducedMotion(true);
    const { result } = renderHook(() => useHeroRotation(titleCount));

    advance(heroRotationIntervalMs * 3);
    expect(result.current.index).toBe(0);
    expect(result.current.rotating).toBe(false);
    expect(result.current.canRotate).toBe(false);

    act(() => {
      result.current.select(1);
    });
    expect(result.current.index).toBe(1);
  });

  it("does not rotate a single title and keeps the index inside a shorter list", () => {
    const { result, rerender } = renderHook(({ count }) => useHeroRotation(count), {
      initialProps: { count: titleCount },
    });
    act(() => {
      result.current.select(2);
    });

    rerender({ count: 1 });
    expect(result.current.index).toBe(0);
    expect(result.current.rotating).toBe(false);
    expect(result.current.canRotate).toBe(false);

    rerender({ count: 0 });
    expect(result.current.index).toBe(0);
  });
});
