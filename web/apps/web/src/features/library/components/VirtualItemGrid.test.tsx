import type { LetterIndex } from "@mivo/api-client";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { useMemo, useState, type ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import { setObservedWidth } from "../../../test/resizeObserver";
import { fakeItems } from "../../../test/libraryFixtures";
import type { LibraryItems } from "../hooks/useLibraryItems";
import type { GridView } from "../lib/gridGeometry";
import { libraryLocales, libraryNamespace } from "../locales";
import { VirtualItemGrid, type VirtualItemGridProps } from "./VirtualItemGrid";

// With the fake observer's 1000 px width, 160 px cards and a 16 px gap the grid has five columns
// of 187.2 px; a poster row is 337 px high plus the 24 px gap, and the window is 768 px high.
const columnsAt1000 = 5;
const rowStrideAt1000 = 361;
const pageSize = 120;

function itemsState(loaded: number, overrides: Partial<LibraryItems> = {}): LibraryItems {
  return {
    status: "success",
    notSupported: false,
    items: fakeItems(loaded),
    totalCount: loaded,
    hasMore: false,
    loadingMore: false,
    loadMoreFailed: false,
    failureCount: 0,
    loadMore: () => undefined,
    retry: () => undefined,
    ...overrides,
  };
}

const gridView: GridView = { viewStyle: "grid", cardStep: 2 };

function baseProps(items: LibraryItems): VirtualItemGridProps {
  return {
    items,
    label: "Videos",
    loadMoreFailedText: "More videos could not be loaded.",
    loadingMoreText: "Loading more videos",
    renderCard: ({ item }) => <a href={`#${item.id}`}>{item.title}</a>,
    view: gridView,
    cardFormat: undefined,
    letters: undefined,
    scrollKey: undefined,
  };
}

async function renderInRouter(content: ReactNode): Promise<HTMLElement> {
  const rootRoute = createRootRoute({ component: () => <main>{content}</main> });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const { container } = render(
    <I18nextProvider i18n={createI18n(["en"], { [libraryNamespace]: libraryLocales })}>
      <RouterProvider router={router} />
    </I18nextProvider>,
  );
  await screen.findByRole("main");
  return container;
}

function scrollWindowTo(top: number): void {
  Object.defineProperty(window, "scrollY", { value: top, configurable: true, writable: true });
  fireEvent.scroll(window);
}

afterEach(() => {
  vi.restoreAllMocks();
  Object.defineProperty(window, "scrollY", { value: 0, configurable: true, writable: true });
});

const bigListing = itemsState(pageSize, { totalCount: 50_000, hasMore: true });

describe("VirtualItemGrid virtualisation", () => {
  it("keeps only the rows near the window in the DOM for 50,000 items", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);

    const list = await screen.findByRole("list", { name: "Videos" });
    const cells = within(list).getAllByRole("listitem", { hidden: true });
    expect(cells.length).toBeGreaterThan(columnsAt1000);
    expect(cells.length).toBeLessThan(60);
    expect(list.style.height).toBe(`${String(10_000 * rowStrideAt1000 - 24)}px`);
  });

  it("describes the position in the whole listing for assistive technology", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);

    const first = within(await screen.findByRole("list", { name: "Videos" })).getAllByRole(
      "listitem",
    )[0];
    expect(first?.getAttribute("aria-posinset")).toBe("1");
    expect(first?.getAttribute("aria-setsize")).toBe("50000");
  });

  it("shows skeleton cards for rows whose page is not loaded", async () => {
    await renderInRouter(
      <VirtualItemGrid {...baseProps(itemsState(8, { totalCount: 50, hasMore: true }))} />,
    );

    const list = await screen.findByRole("list", { name: "Videos" });
    expect(within(list).getAllByRole("link")).toHaveLength(8);
    expect(list.querySelectorAll("li[aria-hidden='true']")).toHaveLength(22);
  });

  it("mounts the rows that scrolling reaches and unmounts the ones left behind", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const list = await screen.findByRole("list", { name: "Videos" });

    act(() => {
      scrollWindowTo(10 * rowStrideAt1000);
    });

    expect(within(list).queryByRole("link", { name: "Item 1" })).toBeNull();
    expect(within(list).getByRole("link", { name: "Item 51" })).toBeTruthy();
  });

  it("requests the next page when the last loaded rows come into view", async () => {
    const loadMore = vi.fn();
    await renderInRouter(<VirtualItemGrid {...baseProps({ ...bigListing, loadMore })} />);
    await screen.findByRole("list", { name: "Videos" });
    expect(loadMore).not.toHaveBeenCalled();

    act(() => {
      scrollWindowTo(19 * rowStrideAt1000);
    });

    expect(loadMore).toHaveBeenCalled();
  });

  it("does not request pages while one loads or after one failed", async () => {
    const loadMore = vi.fn();
    await renderInRouter(
      <VirtualItemGrid
        {...baseProps({ ...bigListing, loadMore, loadMoreFailed: true })}
        scrollKey={undefined}
      />,
    );
    await screen.findByRole("list", { name: "Videos" });
    act(() => {
      scrollWindowTo(19 * rowStrideAt1000);
    });

    expect(loadMore).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe("More videos could not be loaded.");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("lays the columns out again when the width changes", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const list = await screen.findByRole("list", { name: "Videos" });
    const leftOfSecond = () =>
      parseFloat(within(list).getAllByRole("listitem")[1]?.style.left ?? "");
    expect(leftOfSecond()).toBeCloseTo(203.2, 5);

    act(() => {
      setObservedWidth(500);
    });

    // Two columns of 242 px.
    expect(leftOfSecond()).toBeCloseTo(258, 5);
  });
});

describe("VirtualItemGrid view styles", () => {
  it("lays List and Details out as one column of rows", async () => {
    await renderInRouter(
      <VirtualItemGrid {...baseProps(bigListing)} view={{ viewStyle: "list", cardStep: 2 }} />,
    );

    const list = await screen.findByRole("list", { name: "Videos" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows[1]?.style.top).toBe("44px");
    expect(rows[1]?.style.left).toBe("0px");
    const firstRow = rows[0];
    if (firstRow === undefined) {
      throw new Error("The list has no rows.");
    }
    expect(within(firstRow).getByRole("link", { name: "Item 1" })).toBeTruthy();
    expect(list.style.height).toBe(`${String(50_000 * 44)}px`);
  });

  it("makes the cards of a smaller size step narrower", async () => {
    await renderInRouter(
      <VirtualItemGrid {...baseProps(bigListing)} view={{ viewStyle: "grid", cardStep: 0 }} />,
    );

    const list = await screen.findByRole("list", { name: "Videos" });
    const rowSize = list.querySelectorAll("li[style*='top: 0px']").length;
    // 120 px columns with 16 px gaps: seven per 1000 px.
    expect(rowSize).toBe(7);
  });

  it("is a plain accessible list", async () => {
    const container = await renderInRouter(
      <VirtualItemGrid
        {...baseProps(itemsState(30, { totalCount: 60, hasMore: true }))}
        letters={letters}
      />,
    );
    await screen.findByRole("list", { name: "Videos" });

    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});

const letters: LetterIndex = {
  totalCount: 1000,
  letters: [
    { letter: "#", offset: 0 },
    { letter: "B", offset: 40 },
    { letter: "C", offset: 300 },
  ],
};

// A listing that pages like the server: every loadMore adds one page of items.
function PagingGrid({ total }: { readonly total: number }) {
  const [loaded, setLoaded] = useState(pageSize);
  const items = useMemo(
    () =>
      itemsState(loaded, {
        totalCount: total,
        hasMore: loaded < total,
        loadMore: () => {
          setLoaded((count) => Math.min(total, count + pageSize));
        },
      }),
    [loaded, total],
  );
  return <VirtualItemGrid {...baseProps(items)} letters={letters} />;
}

describe("A-Z bar", () => {
  it("dims the letters without titles and leaves them out of the tab order", async () => {
    await renderInRouter(<PagingGrid total={1000} />);

    const bar = await screen.findByRole("navigation", { name: "Jump to letter" });
    expect(within(bar).getAllByRole("button")).toHaveLength(27);
    expect(within(bar).getByRole("button", { name: "Numbers and symbols" })).toBeTruthy();
    expect(within(bar).getByRole("button", { name: "A" }).hasAttribute("disabled")).toBe(true);
    expect(within(bar).getByRole("button", { name: "B" }).hasAttribute("disabled")).toBe(false);
  });

  it("scrolls a loaded letter to the top of the window and focuses its first card", async () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    await renderInRouter(<PagingGrid total={1000} />);

    fireEvent.click(await screen.findByRole("button", { name: "B" }));

    // Item 41 starts row 8; the 72 px clearance keeps it below the sticky top bar.
    expect(scrollTo).toHaveBeenCalledWith({ top: 8 * rowStrideAt1000 - 72 });
    const first = await screen.findByRole("link", { name: "Item 41" });
    expect(document.activeElement).toBe(first);
  });

  it("loads the pages up to a letter before it scrolls there", async () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    await renderInRouter(<PagingGrid total={1000} />);

    fireEvent.click(await screen.findByRole("button", { name: "C" }));

    // Offset 300 lies in the third page of 120.
    await vi.waitFor(() => {
      expect(scrollTo).toHaveBeenCalledWith({ top: 60 * rowStrideAt1000 - 72 });
    });
  });
});

describe("keyboard", () => {
  async function focusFirstCard(): Promise<HTMLElement> {
    const first = await screen.findByRole("link", { name: "Item 1" });
    act(() => {
      first.focus();
    });
    return first;
  }

  it.each([
    ["ArrowRight", "Item 2"],
    ["ArrowDown", "Item 6"],
    ["PageDown", "Item 11"],
  ])("%s moves the focus to %s", async (key, title) => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const first = await focusFirstCard();

    fireEvent.keyDown(first, { key });

    const target = await screen.findByRole("link", { name: title });
    expect(document.activeElement).toBe(target);
  });

  it("does not move the focus past the first card", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const first = await focusFirstCard();

    fireEvent.keyDown(first, { key: "ArrowLeft" });
    fireEvent.keyDown(first, { key: "ArrowUp" });

    expect(document.activeElement).toBe(first);
  });

  it("leaves other keys and shortcuts to the browser", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const first = await focusFirstCard();

    const typed = fireEvent.keyDown(first, { key: "a" });
    const shortcut = fireEvent.keyDown(first, { key: "ArrowRight", ctrlKey: true });

    expect(typed && shortcut).toBe(true);
    expect(document.activeElement).toBe(first);
  });

  it("keeps the focused card in the DOM while the window scrolls away", async () => {
    await renderInRouter(<VirtualItemGrid {...baseProps(bigListing)} />);
    const first = await focusFirstCard();

    act(() => {
      scrollWindowTo(30 * rowStrideAt1000);
    });

    expect(screen.getByRole("link", { name: "Item 1" })).toBe(first);
    expect(document.activeElement).toBe(first);
  });

  it("moves in single steps through the rows of the List view", async () => {
    await renderInRouter(
      <VirtualItemGrid {...baseProps(bigListing)} view={{ viewStyle: "list", cardStep: 2 }} />,
    );
    const first = await focusFirstCard();

    fireEvent.keyDown(first, { key: "ArrowRight" });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(first, { key: "ArrowDown" });

    expect(document.activeElement).toBe(await screen.findByRole("link", { name: "Item 2" }));
  });
});
