import type { ItemSummary } from "@mivo/api-client";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { fakeItem, fakeItems } from "../../../test/libraryFixtures";
import type { LibraryItems } from "./useLibraryItems";
import { useNewPageFocus } from "./useNewPageFocus";

function itemsState(items: readonly ItemSummary[], overrides: Partial<LibraryItems> = {}) {
  return {
    status: "success",
    items,
    hasMore: true,
    loadingMore: false,
    loadMoreFailed: false,
    failureCount: 0,
    loadMore: () => undefined,
    retry: () => undefined,
    ...overrides,
  } satisfies LibraryItems;
}

function renderFocus(initial: LibraryItems) {
  return renderHook((state: LibraryItems) => useNewPageFocus(state), { initialProps: initial });
}

const firstPage = fakeItems(3);
const secondPage = [fakeItem({ id: "item-4" }), fakeItem({ id: "item-5" })];

describe("useNewPageFocus", () => {
  it("targets the first item of the requested page once it arrives", () => {
    const { result, rerender } = renderFocus(itemsState(firstPage));

    act(() => {
      result.current.markLoadMore();
    });
    rerender(itemsState(firstPage, { loadingMore: true }));
    expect(result.current.focusItemId).toBeUndefined();
    rerender(itemsState([...firstPage, ...secondPage]));

    expect(result.current.focusItemId).toBe("item-4");
  });

  it("is not shifted by items a refresh adds above", () => {
    const { result, rerender } = renderFocus(itemsState(firstPage));
    act(() => {
      result.current.markLoadMore();
    });

    rerender(itemsState([fakeItem({ id: "new-on-top" }), ...firstPage, ...secondPage]));

    expect(result.current.focusItemId).toBe("item-4");
  });

  it("waits while the last item at the click is missing", () => {
    const { result, rerender } = renderFocus(itemsState(firstPage));
    act(() => {
      result.current.markLoadMore();
    });

    rerender(itemsState(secondPage));

    expect(result.current.focusItemId).toBeUndefined();
  });

  it("drops the request on a new failure and keeps it during the retry", () => {
    const { result, rerender } = renderFocus(itemsState(firstPage));
    act(() => {
      result.current.markLoadMore();
    });
    rerender(itemsState(firstPage, { loadMoreFailed: true, failureCount: 1 }));
    // The request is gone: a later refresh that brings the page does not move focus.
    rerender(itemsState([...firstPage, ...secondPage]));
    expect(result.current.focusItemId).toBeUndefined();

    rerender(itemsState(firstPage, { loadMoreFailed: true, failureCount: 1 }));
    act(() => {
      result.current.markLoadMore();
    });
    rerender(itemsState(firstPage, { loadMoreFailed: true, failureCount: 1 }));
    rerender(itemsState([...firstPage, ...secondPage]));

    expect(result.current.focusItemId).toBe("item-4");
  });
});
