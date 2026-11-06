import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { usePinEntry, type PinEntryOptions } from "./usePinEntry";

function renderPinEntry(options: Partial<PinEntryOptions> = {}) {
  const onComplete = vi.fn<PinEntryOptions["onComplete"]>();
  const rendered = renderHook(() => usePinEntry({ onComplete, busy: false, ...options }));
  return { ...rendered, onComplete };
}

describe("usePinEntry", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("submits on the 4th digit and never earlier", () => {
    const { result, onComplete } = renderPinEntry();

    act(() => {
      result.current.press(1);
      result.current.press(2);
      result.current.press(3);
    });
    expect(result.current.length).toBe(3);
    expect(onComplete).not.toHaveBeenCalled();

    act(() => {
      result.current.press(4);
    });
    expect(onComplete).toHaveBeenCalledExactlyOnceWith("1234", expect.any(Function));
  });

  it("deletes digits and submits a whole PIN on OK again", () => {
    const { result, onComplete } = renderPinEntry();

    act(() => {
      result.current.press(9);
      result.current.press("delete");
      result.current.press("submit");
    });
    expect(result.current.length).toBe(0);
    expect(onComplete).not.toHaveBeenCalled();

    act(() => {
      for (const digit of [5, 6, 7, 8]) {
        result.current.press(digit);
      }
      result.current.press(9);
      result.current.press("submit");
    });
    expect(onComplete.mock.calls.map(([pin]) => pin)).toEqual(["5678", "5678"]);
  });

  it("clears the digits and counts a wrong PIN", () => {
    const { result } = renderPinEntry();
    act(() => {
      result.current.press(1);
      result.current.reject({ kind: "wrongPin" });
    });

    expect(result.current.length).toBe(0);
    expect(result.current.wrongCount).toBe(1);
    expect(result.current.failure).toEqual({ kind: "wrongPin" });

    act(() => {
      result.current.press(2);
    });
    expect(result.current.failure).toBeUndefined();
  });

  it("ignores keys during a lockout and counts it down", () => {
    const { result, onComplete } = renderPinEntry();
    act(() => {
      result.current.reject({ kind: "locked", seconds: 2 });
    });
    expect(result.current.blocked).toBe(true);
    expect(result.current.lockout.secondsLeft).toBe(2);

    act(() => {
      result.current.press(1);
    });
    expect(result.current.length).toBe(0);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.lockout.secondsLeft).toBe(1);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.blocked).toBe(false);
    expect(result.current.lockout.over).toBe(true);

    act(() => {
      for (const digit of [1, 2, 3, 4]) {
        result.current.press(digit);
      }
    });
    expect(onComplete).toHaveBeenCalledExactlyOnceWith("1234", expect.any(Function));
    expect(result.current.lockout.over).toBe(false);
  });

  it("waits while a check runs", () => {
    const { result } = renderPinEntry({ busy: true });
    act(() => {
      result.current.press(1);
    });
    expect(result.current.length).toBe(0);
  });

  it("starts over on reset", () => {
    const { result } = renderPinEntry();
    act(() => {
      result.current.press(1);
      result.current.reject({ kind: "locked", seconds: 60 });
      result.current.reset();
    });
    expect(result.current.blocked).toBe(false);
    expect(result.current.failure).toBeUndefined();
  });
});
