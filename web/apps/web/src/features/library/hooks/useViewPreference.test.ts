import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { cardStepStorageKey, viewStyleStorageKey } from "../lib/viewStyle";
import { useViewPreference } from "./useViewPreference";

beforeEach(() => {
  window.localStorage.clear();
});

describe("useViewPreference", () => {
  it("starts with the grid at the default step", () => {
    const { result } = renderHook(() => useViewPreference(window.localStorage));
    expect(result.current.viewStyle).toBe("grid");
    expect(result.current.cardStep).toBe(2);
  });

  it("remembers the view and the step on the device", () => {
    const first = renderHook(() => useViewPreference(window.localStorage));
    act(() => {
      first.result.current.selectViewStyle("list");
      first.result.current.selectCardStep(4);
    });
    expect(window.localStorage.getItem(viewStyleStorageKey)).toBe("list");
    expect(window.localStorage.getItem(cardStepStorageKey)).toBe("4");

    const second = renderHook(() => useViewPreference(window.localStorage));
    expect(second.result.current.viewStyle).toBe("list");
    expect(second.result.current.cardStep).toBe(4);
  });

  it("ignores unknown view ids and clamps the step", () => {
    const { result } = renderHook(() => useViewPreference(window.localStorage));
    act(() => {
      result.current.selectViewStyle("cover");
      result.current.selectCardStep(42);
    });
    expect(result.current.viewStyle).toBe("grid");
    expect(result.current.cardStep).toBe(4);
  });

  it("works without storage", () => {
    const { result } = renderHook(() => useViewPreference(undefined));
    act(() => {
      result.current.selectViewStyle("details");
    });
    expect(result.current.viewStyle).toBe("details");
  });
});
