import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { stubMediaQueries } from "../../test/mediaQuery";
import { useMediaQuery } from "./useMediaQuery";

const query = "(width < 48rem)";

describe("useMediaQuery", () => {
  it("counts as not matching when matchMedia is missing", () => {
    const { result } = renderHook(() => useMediaQuery(query));

    expect(result.current).toBe(false);
  });

  it("follows change events", () => {
    const queries = stubMediaQueries(false);
    const { result } = renderHook(() => useMediaQuery(query));
    expect(result.current).toBe(false);
    act(() => {
      queries.setMatches(true);
    });

    expect(result.current).toBe(true);
  });

  it("removes its change listener on unmount", () => {
    stubMediaQueries(false);
    const remove = vi.spyOn(EventTarget.prototype, "removeEventListener");
    try {
      const { unmount } = renderHook(() => useMediaQuery(query));
      unmount();

      expect(remove).toHaveBeenCalledWith("change", expect.any(Function));
    } finally {
      remove.mockRestore();
    }
  });
});
