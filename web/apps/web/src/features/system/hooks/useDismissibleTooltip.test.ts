import { act, fireEvent, renderHook } from "@testing-library/react";
import { describe, expect, it, onTestFinished, vi } from "vitest";

import { useDismissibleTooltip } from "./useDismissibleTooltip";

function renderTooltip() {
  return renderHook(() => useDismissibleTooltip());
}

function pressOnDocument(key: string): void {
  act(() => {
    fireEvent.keyDown(document, { key });
  });
}

describe("useDismissibleTooltip", () => {
  it("starts closed", () => {
    const { result } = renderTooltip();

    expect(result.current.open).toBe(false);
  });

  it("opens on hover and stays open until the pointer leaves", () => {
    const { result } = renderTooltip();

    act(() => {
      result.current.triggerHandlers.onMouseEnter();
    });
    expect(result.current.open).toBe(true);

    act(() => {
      result.current.triggerHandlers.onMouseLeave();
    });
    expect(result.current.open).toBe(false);
  });

  it("opens on focus and stays open until the trigger loses focus", () => {
    const { result } = renderTooltip();

    act(() => {
      result.current.triggerHandlers.onFocus();
    });
    expect(result.current.open).toBe(true);

    act(() => {
      result.current.triggerHandlers.onBlur();
    });
    expect(result.current.open).toBe(false);
  });

  it("stays open while still focused after the pointer leaves", () => {
    const { result } = renderTooltip();

    act(() => {
      result.current.triggerHandlers.onFocus();
      result.current.triggerHandlers.onMouseEnter();
    });
    act(() => {
      result.current.triggerHandlers.onMouseLeave();
    });

    expect(result.current.open).toBe(true);
  });

  it("closes on Escape while hovered or focused", () => {
    const { result } = renderTooltip();
    act(() => {
      result.current.triggerHandlers.onFocus();
      result.current.triggerHandlers.onMouseEnter();
    });

    pressOnDocument("Escape");

    expect(result.current.open).toBe(false);
  });

  it("ignores other keys", () => {
    const { result } = renderTooltip();
    act(() => {
      result.current.triggerHandlers.onMouseEnter();
    });

    pressOnDocument("Enter");

    expect(result.current.open).toBe(true);
  });

  it("closes on Escape even when a handler stops it from bubbling", () => {
    const { result } = renderTooltip();
    const swallowEscape = (event: KeyboardEvent) => {
      event.stopPropagation();
    };
    document.body.addEventListener("keydown", swallowEscape);
    onTestFinished(() => {
      document.body.removeEventListener("keydown", swallowEscape);
    });
    act(() => {
      result.current.triggerHandlers.onMouseEnter();
    });

    act(() => {
      fireEvent.keyDown(document.body, { key: "Escape" });
    });

    expect(result.current.open).toBe(false);
  });

  it.each([
    ["hovered", "onMouseEnter", "onMouseLeave"],
    ["focused", "onFocus", "onBlur"],
  ] as const)("opens again when %s anew after Escape", (_case, enter, leave) => {
    const { result } = renderTooltip();
    act(() => {
      result.current.triggerHandlers[enter]();
    });
    pressOnDocument("Escape");

    act(() => {
      result.current.triggerHandlers[leave]();
    });
    act(() => {
      result.current.triggerHandlers[enter]();
    });

    expect(result.current.open).toBe(true);
  });

  it("listens for Escape only while open", () => {
    const addListener = vi.spyOn(document, "addEventListener");
    const removeListener = vi.spyOn(document, "removeEventListener");
    onTestFinished(() => {
      addListener.mockRestore();
      removeListener.mockRestore();
    });
    const { result, unmount } = renderTooltip();
    const keydownCalls = (calls: readonly (readonly unknown[])[]) =>
      calls.filter(([type]) => type === "keydown").length;

    expect(keydownCalls(addListener.mock.calls)).toBe(0);
    act(() => {
      result.current.triggerHandlers.onMouseEnter();
    });
    expect(keydownCalls(addListener.mock.calls)).toBe(1);
    pressOnDocument("Escape");
    expect(keydownCalls(removeListener.mock.calls)).toBe(1);
    unmount();
    expect(keydownCalls(removeListener.mock.calls)).toBe(1);
  });
});
