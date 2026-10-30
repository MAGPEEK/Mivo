import type { ItemSummary, Library, ScanStatus } from "@mivo/api-client";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  respondNever,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import {
  fakeItem,
  fakeItems,
  fakeLibrary,
  finishedScan,
  itemsRoute,
  librariesRoute,
  libraryRoute,
  rootUnavailableScan,
  runningScan,
} from "../../../test/libraryFixtures";
import { renderApp, type RenderedApp } from "../../../test/renderApp";
import { libraryItemsPageSize, scanStatusPollIntervalMs } from "../api/libraryQueries";

const library = fakeLibrary();
const libraryPath = `/libraries/${library.id}`;
const libraryApiPath = `/api/v1/libraries/${library.id}`;
const itemsApiPath = `${libraryApiPath}/items`;

interface RenderedLibrary extends RenderedApp {
  readonly server: FakeServer;
}

function renderLibrary(
  routes: readonly FakeRoute[],
  languages: readonly string[] = ["en"],
): RenderedLibrary {
  const server = createFakeServer(routes);
  return { ...renderApp(libraryPath, { fetch: server.fetch, languages }), server };
}

function renderLoadedLibrary(
  items: readonly ItemSummary[] = [fakeItem()],
  libraryOverrides: Partial<Library> = {},
): RenderedLibrary {
  const shown = { ...library, ...libraryOverrides };
  return renderLibrary([libraryRoute(shown), itemsRoute(library.id, items)]);
}

async function findItemList(): Promise<HTMLElement> {
  return screen.findByRole("list", { name: "Videos" });
}

function scanButton(): HTMLElement {
  return screen.getByRole("button", { name: /^(Scan library|Scanning…)$/ });
}

// The status line next to the scan button; the grid has its own loading status.
function scanStatus(): HTMLElement {
  return within(scanButton().parentElement ?? document.body).getByRole("status");
}

describe("LibraryPage header", () => {
  it("shows the library name and its item count", async () => {
    renderLoadedLibrary();

    expect(await screen.findByRole("heading", { level: 1, name: "Movies" })).toBeTruthy();
    expect(screen.getByText("2 videos")).toBeTruthy();
    expect(screen.queryByText("Folder unavailable")).toBeNull();
  });

  it("formats large counts for the language", async () => {
    renderLoadedLibrary([fakeItem()], { itemCount: 1234 });

    expect(await screen.findByText("1,234 videos")).toBeTruthy();
  });

  it("uses the singular for one item", async () => {
    renderLoadedLibrary([fakeItem()], { itemCount: 1 });

    expect(await screen.findByText("1 video")).toBeTruthy();
  });

  it("marks a library whose folder is unavailable", async () => {
    renderLoadedLibrary([fakeItem()], { available: false });

    expect(await screen.findByText("Folder unavailable")).toBeTruthy();
  });

  it("shows a generic heading while the library loads", async () => {
    renderApp(libraryPath, { fetch: respondNever() });

    expect(await screen.findByRole("heading", { level: 1, name: "Library" })).toBeTruthy();
    expect(within(screen.getByRole("main")).getByRole("status").textContent).toBe(
      "Loading library",
    );
  });
});

describe("LibraryPage errors", () => {
  it("explains an unknown library and links back home", async () => {
    const { server, router } = renderLibrary([]);

    expect(await screen.findByText("This library does not exist.")).toBeTruthy();
    expect(server.countRequests("GET", libraryApiPath)).toBe(1);
    fireEvent.click(screen.getByRole("link", { name: "Back to home" }));
    await screen.findByRole("heading", { level: 1, name: "Home" });
    expect(router.state.location.pathname).toBe("/");
  });

  it("retries a failed load once, then offers to try again", async () => {
    let failing = true;
    const { server } = renderLibrary([
      {
        method: "GET",
        path: libraryApiPath,
        respond: () => (failing ? problemResponse(500, "internal") : jsonResponse(library)),
      },
      itemsRoute(library.id, []),
    ]);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "The library could not be loaded. Something went wrong. Try again later.",
    );
    expect(server.countRequests("GET", libraryApiPath)).toBe(2);

    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Movies" })).toBeTruthy();
  });

  it("explains an unreachable server", async () => {
    renderLibrary([
      { method: "GET", path: libraryApiPath, respond: () => Promise.reject(new TypeError("x")) },
    ]);

    expect((await screen.findByRole("alert")).textContent).toBe(
      "The library could not be loaded. The server cannot be reached.",
    );
  });
});

describe("LibraryPage grid", () => {
  it("lists every item as a labelled link with duration and technical chip", async () => {
    renderLoadedLibrary([fakeItem(), fakeItem({ id: "item-2", title: "Sintel" })]);

    const list = await findItemList();
    const links = within(list).getAllByRole("link");

    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    expect(links.map((link) => link.textContent)).toEqual(["Big Buck Bunny", "Sintel"]);
    const details = document.getElementById(links[0]?.getAttribute("aria-describedby") ?? "");
    expect(details?.textContent).toBe("9:56MKV · H.264 · AAC");
  });

  it("shows the title initial on the placeholder poster, hidden from screen readers", async () => {
    renderLoadedLibrary([fakeItem({ title: "sintel" })]);

    const listItem = within(await findItemList()).getByRole("listitem");
    const poster = within(listItem).getByText("S");

    expect(poster.getAttribute("aria-hidden")).toBe("true");
  });

  it("greys out an unavailable item and says so in its description", async () => {
    renderLoadedLibrary([fakeItem({ title: "Broken", media: { available: false } })]);

    const link = within(await findItemList()).getByRole("link", { name: "Broken" });
    const details = document.getElementById(link.getAttribute("aria-describedby") ?? "");

    expect(details?.textContent).toBe("Unavailable");
  });

  it("opens the item page when a card is clicked", async () => {
    const { router } = renderLoadedLibrary();

    fireEvent.click(within(await findItemList()).getByRole("link", { name: "Big Buck Bunny" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/items/${fakeItem().id}`);
    });
  });

  it("pages with Load more and moves focus to the first new card", async () => {
    const items = fakeItems(libraryItemsPageSize * 2 + 1);
    const { server } = renderLoadedLibrary(items);

    const list = await findItemList();
    expect(within(list).getAllByRole("listitem")).toHaveLength(libraryItemsPageSize);
    const firstRequest = new URL(server.requests.find((r) => r.url.includes("/items"))?.url ?? "");
    expect(firstRequest.searchParams.get("limit")).toBe(String(libraryItemsPageSize));
    expect(firstRequest.searchParams.has("cursor")).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    const firstNew = await within(list).findByRole("link", {
      name: `Item ${String(libraryItemsPageSize + 1)}`,
    });
    expect(document.activeElement).toBe(firstNew);

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    const last = await within(list).findByRole("link", { name: `Item ${String(items.length)}` });
    expect(document.activeElement).toBe(last);
    expect(within(list).getAllByRole("listitem")).toHaveLength(items.length);
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
    expect(server.countRequests("GET", itemsApiPath)).toBe(3);
  });

  it("keeps Load more focused and ignores clicks while the next page loads", async () => {
    let releaseSecondPage: () => void = () => undefined;
    const pagedItems = itemsRoute(library.id, fakeItems(libraryItemsPageSize + 1));
    const { server } = renderLibrary([
      libraryRoute(library),
      {
        ...pagedItems,
        respond: async (request) => {
          if (request.url.searchParams.has("cursor")) {
            await new Promise<void>((resolve) => {
              releaseSecondPage = resolve;
            });
          }
          return pagedItems.respond(request);
        },
      },
    ]);
    await findItemList();
    const button = screen.getByRole("button", { name: "Load more" });
    act(() => {
      button.focus();
    });

    fireEvent.click(button);
    const busy = await screen.findByRole("button", { name: "Loading more videos" });
    fireEvent.click(busy);

    expect(busy).toBe(button);
    expect(busy.getAttribute("aria-busy")).toBe("true");
    expect(busy.getAttribute("aria-disabled")).toBe("true");
    expect(document.activeElement).toBe(busy);
    expect(server.countRequests("GET", itemsApiPath)).toBe(2);
    releaseSecondPage();
    expect(await screen.findByRole("link", { name: "Item 61" })).toBeTruthy();
  });

  it("still shows the next page when a scan starts while it loads", async () => {
    let releaseSecondPage: () => void = () => undefined;
    const pagedItems = itemsRoute(library.id, fakeItems(libraryItemsPageSize + 1));
    const { server } = renderLibrary([
      libraryRoute(library),
      {
        ...pagedItems,
        respond: async (request) => {
          if (request.url.searchParams.has("cursor")) {
            await new Promise<void>((resolve) => {
              releaseSecondPage = resolve;
            });
          }
          return pagedItems.respond(request);
        },
      },
      {
        method: "POST",
        path: `${libraryApiPath}/scan`,
        respond: () => jsonResponse(runningScan, 202),
      },
    ]);
    await findItemList();
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    await screen.findByRole("button", { name: "Loading more videos" });

    fireEvent.click(scanButton());
    await waitFor(() => {
      expect(scanButton().textContent).toBe("Scanning…");
    });
    await waitFor(() => {
      expect(server.countRequests("POST", `${libraryApiPath}/scan`)).toBe(1);
    });
    releaseSecondPage();

    expect(await screen.findByRole("link", { name: "Item 61" })).toBeTruthy();
    expect(server.countRequests("GET", itemsApiPath)).toBe(2);
  });

  it("keeps the cards when the next page fails and retries it in place", async () => {
    let failing = true;
    const pagedItems = itemsRoute(library.id, fakeItems(libraryItemsPageSize + 1));
    renderLibrary([
      libraryRoute(library),
      {
        ...pagedItems,
        respond: (request) =>
          failing && request.url.searchParams.has("cursor")
            ? problemResponse(500, "internal")
            : pagedItems.respond(request),
      },
    ]);
    const list = await findItemList();
    const button = screen.getByRole("button", { name: "Load more" });
    act(() => {
      button.focus();
    });

    fireEvent.click(button);

    expect((await screen.findByRole("alert")).textContent).toBe("More videos could not be loaded.");
    expect(within(list).getAllByRole("listitem")).toHaveLength(libraryItemsPageSize);
    expect(button.textContent).toBe("Try again");
    expect(document.activeElement).toBe(button);

    failing = false;
    fireEvent.click(button);

    const firstNew = await within(list).findByRole("link", { name: "Item 61" });
    expect(document.activeElement).toBe(firstNew);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("does not move focus again when a refresh adds cards later", async () => {
    let shown = fakeItems(libraryItemsPageSize + 1);
    const { server } = renderLibrary([
      libraryRoute(library),
      {
        method: "POST",
        path: `${libraryApiPath}/scan`,
        respond: () => {
          // Lands exactly where the page loaded by "Load more" began.
          shown = [
            ...shown.slice(0, libraryItemsPageSize),
            fakeItem({ id: "new", title: "New arrival" }),
            ...shown.slice(libraryItemsPageSize),
          ];
          return jsonResponse(finishedScan, 202);
        },
      },
      {
        method: "GET",
        path: itemsApiPath,
        respond: (request) => itemsRoute(library.id, shown).respond(request),
      },
    ]);
    const list = await findItemList();
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    const firstNew = await within(list).findByRole("link", { name: "Item 61" });
    expect(document.activeElement).toBe(firstNew);

    act(() => {
      scanButton().focus();
    });
    fireEvent.click(scanButton());

    expect(await within(list).findByRole("link", { name: "New arrival" })).toBeTruthy();
    await waitFor(() => {
      expect(server.countRequests("GET", itemsApiPath)).toBe(4);
    });
    expect(document.activeElement).toBe(scanButton());
  });

  it("keeps the cards when a background refresh fails", async () => {
    let failing = false;
    const items = itemsRoute(library.id, [fakeItem()]);
    const { server } = renderLibrary([
      libraryRoute(library),
      {
        method: "POST",
        path: `${libraryApiPath}/scan`,
        respond: () => {
          failing = true;
          return jsonResponse(finishedScan, 202);
        },
      },
      {
        ...items,
        respond: (request) => (failing ? problemResponse(500, "internal") : items.respond(request)),
      },
    ]);
    await findItemList();

    fireEvent.click(scanButton());

    // First load, then the refresh after the scan and its one retry.
    await waitFor(() => {
      expect(server.countRequests("GET", itemsApiPath)).toBe(3);
    });
    await waitFor(() => {
      expect(scanButton().getAttribute("aria-busy")).toBe("false");
    });
    expect(screen.getByRole("link", { name: "Big Buck Bunny" })).toBeTruthy();
    expect(screen.queryByText("The videos could not be loaded.")).toBeNull();
  });

  it("asks for a scan when the library has no items", async () => {
    renderLoadedLibrary([], { itemCount: 0 });

    expect(await screen.findByRole("heading", { level: 2, name: "No videos yet" })).toBeTruthy();
    expect(
      screen.getByText("Run a scan to find the videos in this library's folder."),
    ).toBeTruthy();
  });

  it("reports items that cannot be loaded and retries on request", async () => {
    let failing = true;
    const items = itemsRoute(library.id, [fakeItem()]);
    renderLibrary([
      libraryRoute(library),
      {
        ...items,
        respond: (request) => (failing ? problemResponse(400, "invalid") : items.respond(request)),
      },
    ]);

    expect((await screen.findByRole("alert")).textContent).toBe("The videos could not be loaded.");
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await findItemList()).toBeTruthy();
  });

  it("shows a loading placeholder while the items load", async () => {
    renderLibrary([
      libraryRoute(library),
      { method: "GET", path: itemsApiPath, respond: () => new Promise<Response>(() => undefined) },
    ]);

    await screen.findByRole("heading", { level: 1, name: "Movies" });

    expect(screen.getByText("Loading videos")).toBeTruthy();
  });
});

describe("LibraryPage scan status", () => {
  it.each([
    { name: "before the first scan", scan: library.scan, expected: "" },
    {
      name: "after a finished scan",
      scan: finishedScan,
      expected: "Last scan: 3 added, 1 updated, 2 removed, 29 unchanged, 1 failed, 4 skipped.",
    },
    {
      name: "after the folder was missing",
      scan: rootUnavailableScan,
      expected: "Scan failed: The library folder is missing or cannot be read.",
    },
    {
      name: "when ffprobe is missing",
      scan: { ...rootUnavailableScan, errorCode: "prober_unavailable" },
      expected:
        "Scan failed: ffprobe was not found. Install FFmpeg or set tools.ffprobe in config.yaml, then restart the server.",
    },
    {
      name: "after an unknown failure",
      scan: { ...rootUnavailableScan, errorCode: "disk_on_fire" },
      expected: "Scan failed: Something went wrong. Try again later.",
    },
  ])("describes the state $name", async ({ scan, expected }) => {
    renderLoadedLibrary([fakeItem()], { scan });
    await screen.findByRole("heading", { level: 1, name: "Movies" });

    expect(scanStatus().textContent).toBe(expected);
    expect(scanButton().getAttribute("aria-disabled")).toBe("false");
  });

  it.each([
    {
      name: "the server is shutting down",
      response: () => problemResponse(503, "unavailable"),
      expected:
        "The scan could not be started: The server is not available right now. Try again in a moment.",
    },
    {
      name: "the server is unreachable",
      response: () => Promise.reject(new TypeError("Failed to fetch")),
      expected: "The scan could not be started: The server cannot be reached.",
    },
  ])("says why a scan could not start when $name", async ({ response, expected }) => {
    renderLibrary([
      libraryRoute(library),
      itemsRoute(library.id, []),
      { method: "POST", path: `${libraryApiPath}/scan`, respond: response },
    ]);
    await screen.findByRole("heading", { level: 1, name: "Movies" });

    fireEvent.click(scanButton());

    expect(await screen.findByText(expected)).toBeTruthy();
    expect(scanButton().getAttribute("aria-disabled")).toBe("false");
  });

  it("refreshes items and libraries when the scan is done before the response", async () => {
    let itemsShown: readonly ItemSummary[] = [];
    const { server } = renderLibrary([
      libraryRoute(library),
      {
        method: "GET",
        path: itemsApiPath,
        respond: (r) => itemsRoute(library.id, itemsShown).respond(r),
      },
      {
        method: "POST",
        path: `${libraryApiPath}/scan`,
        respond: () => {
          itemsShown = [fakeItem()];
          return jsonResponse(finishedScan, 202);
        },
      },
    ]);
    await screen.findByText("No videos yet");

    fireEvent.click(scanButton());

    expect(
      await within(await findItemList()).findByRole("link", { name: "Big Buck Bunny" }),
    ).toBeTruthy();
    expect(server.countRequests("GET", "/api/v1/libraries")).toBe(2);
  });
});

// Only intervals are faked: TanStack Query polls with setInterval, so the test controls the
// poll cadence exactly, while responses, renders and waitFor run on real timers.
describe("LibraryPage scan polling", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  async function advance(milliseconds: number): Promise<void> {
    await act(() => vi.advanceTimersByTimeAsync(milliseconds));
  }

  function scanningServer(initialScan: ScanStatus = library.scan) {
    let current: Library = { ...library, scan: initialScan };
    let itemsShown: readonly ItemSummary[] = [];
    let removed = false;
    const rendered = renderLibrary([
      librariesRoute([library]),
      {
        method: "GET",
        path: libraryApiPath,
        respond: () => (removed ? problemResponse(404, "not_found") : jsonResponse(current)),
      },
      {
        method: "GET",
        path: itemsApiPath,
        respond: (request) => itemsRoute(library.id, itemsShown).respond(request),
      },
      {
        method: "POST",
        path: `${libraryApiPath}/scan`,
        respond: () => {
          current = { ...current, scan: runningScan };
          return jsonResponse(runningScan, 202);
        },
      },
    ]);
    const finish = (scan: ScanStatus) => {
      current = { ...current, scan, itemCount: 1 };
      itemsShown = [fakeItem()];
    };
    const remove = () => {
      removed = true;
    };
    return { ...rendered, finish, remove };
  }

  it("starts a scan, polls every two seconds while it runs and shows the result", async () => {
    const { server, finish } = scanningServer();
    await screen.findByText("No videos yet");

    fireEvent.click(scanButton());

    await waitFor(() => {
      expect(scanStatus().textContent).toBe("Scanning the library folder…");
    });
    expect(server.countRequests("POST", `${libraryApiPath}/scan`)).toBe(1);
    expect(scanButton().textContent).toBe("Scanning…");
    expect(scanButton().getAttribute("aria-busy")).toBe("true");
    expect(scanButton().getAttribute("aria-disabled")).toBe("true");
    const getsAfterStart = server.countRequests("GET", libraryApiPath);

    await advance(scanStatusPollIntervalMs - 1);
    expect(server.countRequests("GET", libraryApiPath)).toBe(getsAfterStart);
    await advance(1);
    expect(server.countRequests("GET", libraryApiPath)).toBe(getsAfterStart + 1);
    await advance(scanStatusPollIntervalMs);
    expect(server.countRequests("GET", libraryApiPath)).toBe(getsAfterStart + 2);

    finish(finishedScan);
    await advance(scanStatusPollIntervalMs);

    await waitFor(() => {
      expect(scanButton().textContent).toBe("Scan library");
    });
    expect(scanButton().getAttribute("aria-busy")).toBe("false");
    expect(scanStatus().textContent).toContain("Last scan: 3 added");
    expect(await screen.findByRole("link", { name: "Big Buck Bunny" })).toBeTruthy();
    expect(await screen.findByText("1 video")).toBeTruthy();
    expect(server.countRequests("GET", "/api/v1/libraries")).toBe(2);
    const getsWhenDone = server.countRequests("GET", libraryApiPath);
    await advance(scanStatusPollIntervalMs * 3);
    expect(server.countRequests("GET", libraryApiPath)).toBe(getsWhenDone);
  });

  it("follows a scan that was already running and shows its failure", async () => {
    const { server, finish } = scanningServer(runningScan);
    await screen.findByText("No videos yet");
    expect(scanButton().getAttribute("aria-disabled")).toBe("true");

    finish(rootUnavailableScan);
    await advance(scanStatusPollIntervalMs);

    await waitFor(() => {
      expect(scanStatus().textContent).toBe(
        "Scan failed: The library folder is missing or cannot be read.",
      );
    });
    expect(scanButton().getAttribute("aria-disabled")).toBe("false");
    expect(server.countRequests("POST", `${libraryApiPath}/scan`)).toBe(0);
  });

  it("stops polling and shows the not-found state when the library disappears", async () => {
    const { server, remove } = scanningServer(runningScan);
    await screen.findByText("No videos yet");

    remove();
    await advance(scanStatusPollIntervalMs);

    expect(await screen.findByText("This library does not exist.")).toBeTruthy();
    expect(screen.queryByRole("heading", { level: 1, name: "Movies" })).toBeNull();
    const getsWhenGone = server.countRequests("GET", libraryApiPath);
    await advance(scanStatusPollIntervalMs * 3);
    expect(server.countRequests("GET", libraryApiPath)).toBe(getsWhenGone);
  });

  it("stops polling when the page is left", async () => {
    const { server, router } = scanningServer(runningScan);
    await screen.findByText("No videos yet");

    await act(() => router.navigate({ to: "/" }));
    await screen.findByRole("heading", { level: 1, name: "Home" });
    const getsWhenLeft = server.countRequests("GET", libraryApiPath);
    await advance(scanStatusPollIntervalMs * 3);

    expect(server.countRequests("GET", libraryApiPath)).toBe(getsWhenLeft);
  });
});

describe("LibraryPage language and accessibility", () => {
  it("renders in German", async () => {
    renderLibrary(
      [
        libraryRoute({ ...library, itemCount: 1234, scan: finishedScan }),
        itemsRoute(library.id, []),
      ],
      ["de"],
    );

    expect(await screen.findByText("1.234 Videos")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Bibliothek scannen" })).toBeTruthy();
    expect(await screen.findByText("Noch keine Videos")).toBeTruthy();
    expect(screen.getByText(/^Letzter Scan: 3 neu/)).toBeTruthy();
  });

  it.each([
    {
      name: "with items",
      items: [fakeItem(), fakeItem({ id: "x", title: "Broken", media: { available: false } })],
    },
    { name: "without items", items: [] },
  ])("has no serious or critical axe findings $name", async ({ items }) => {
    renderLoadedLibrary(items, { available: false, scan: finishedScan });
    await screen.findByRole("heading", { level: 1, name: "Movies" });
    await screen.findByText(items.length === 0 ? "No videos yet" : "Big Buck Bunny");

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
