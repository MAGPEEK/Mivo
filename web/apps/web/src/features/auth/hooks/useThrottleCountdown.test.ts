import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useThrottleCountdown } from "./useThrottleCountdown";

describe("useThrottleCountdown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("counts the wait down to zero and then reports it as over", () => {
    const { result } = renderHook(useThrottleCountdown);

    act(() => {
      result.current.start(3);
    });
    expect(result.current).toMatchObject({ secondsLeft: 3, totalSeconds: 3, throttled: true });

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.secondsLeft).toBe(2);

    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current).toMatchObject({ secondsLeft: 0, throttled: false, over: true });

    act(() => {
      result.current.reset();
    });
    expect(result.current.over).toBe(false);
  });

  it("is idle before any wait", () => {
    const { result } = renderHook(useThrottleCountdown);

    expect(result.current).toMatchObject({ secondsLeft: 0, throttled: false, over: false });
  });
});
