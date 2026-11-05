import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

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

  it("keeps one change listener across rerenders and removes it on unmount", () => {
    const queries = stubMediaQueries(false);
    const { rerender, unmount } = renderHook(() => useMediaQuery(query));
    rerender();

    expect(queries.changeListenerCount()).toBe(1);
    unmount();
    expect(queries.changeListenerCount()).toBe(0);
  });
});
