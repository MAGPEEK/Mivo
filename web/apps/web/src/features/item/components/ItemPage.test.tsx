import type { ItemDetail } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { createFakeServer, jsonResponse } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import { itemQueryKey } from "../api/itemQuery";
import {
  hdrMovie,
  itemServer,
  mediaFile,
  notFoundProblem,
  recordingItemRequests,
  unreadableItem,
  type ItemServerFetch,
} from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";

const subtitleHint = "Subtitles are listed here, but the player does not show them yet.";
const networkErrorText =
  "The server could not be reached. Check that it is running, then try again.";
const serverErrorText = "Something went wrong while loading this item. Try again in a moment.";

async function findMain(): Promise<HTMLElement> {
  return screen.findByRole("main");
}

async function renderItem(
  item: ItemDetail,
  languages: readonly string[] = ["en"],
): Promise<HTMLElement> {
  renderItemPage(`/items/${encodeURIComponent(item.id)}`, {
    fetch: itemServer([item]),
    languages,
  });
  const main = await findMain();
  await within(main).findByRole("heading", { level: 1, name: item.title });
  return main;
}

function listItems(container: HTMLElement, name: string): string[] {
  return within(within(container).getByRole("list", { name }))
    .getAllByRole("listitem")
    .map((element) => element.textContent);
}

function itemWithDuration(durationSeconds: number): ItemDetail {
  return { ...hdrMovie, files: [mediaFile({ durationSeconds, streams: [] })] };
}

const offline: ItemServerFetch = () => Promise.reject(new TypeError("Failed to fetch"));

describe("ItemPage with a playable item", () => {
  it("shows title, facts, technical chips and the file", async () => {
    const main = await renderItem(hdrMovie);

    expect(within(main).getByText("1 h 30 min · 4K")).toBeTruthy();
    expect(listItems(main, "Technical details")).toEqual([
      "MKV",
      "HEVC 10-bit HDR10",
      "AC-3 5.1",
      "SRT",
    ]);
    const file = within(main).getByRole("region", { name: "File" });
    expect(within(file).getByText("Big.Buck.Bunny.mkv")).toBeTruthy();
    expect(within(file).getByText("276.1 MB")).toBeTruthy();
  });

  it("lists audio and subtitle tracks with language, title and flags", async () => {
    const main = await renderItem(hdrMovie);

    const audio = within(main).getByRole("region", { name: "Audio" });
    expect(listItems(audio, "Audio")).toEqual([
      "GermanAC-3 5.1Default",
      "EnglishDirector's commentaryAAC 2.0",
    ]);
    const subtitles = within(main).getByRole("region", { name: "Subtitles" });
    expect(listItems(subtitles, "Subtitles")).toEqual(["EnglishSRTForced", "Unknown languageSRT"]);
    expect(within(subtitles).getByText(subtitleHint)).toBeTruthy();
  });

  it("links Play to the player for the primary file", async () => {
    const { router } = renderItemPage("/items/item-1", { fetch: itemServer([hdrMovie]) });
    const main = await findMain();

    const play = await within(main).findByRole("link", { name: "Play" });
    expect(play.getAttribute("href")).toBe("/items/item-1/play?fileId=file-1");

    fireEvent.click(play);

    expect(await screen.findByRole("heading", { level: 1, name: "Player" })).toBeTruthy();
    expect(router.state.location.pathname).toBe("/items/item-1/play");
    expect(router.state.location.search).toEqual({ fileId: "file-1" });
  });

  it("encodes reserved characters of the IDs in the request and in the Play link", async () => {
    const item: ItemDetail = {
      ...hdrMovie,
      id: "a/b?c",
      files: [mediaFile({ id: "f&1#x" })],
    };
    const server = recordingItemRequests(itemServer([item]));
    renderItemPage(`/items/${encodeURIComponent(item.id)}`, { fetch: server.fetch });
    const main = await findMain();

    const play = await within(main).findByRole("link", { name: "Play" });

    expect(server.itemRequests).toEqual(["/api/v1/items/a%2Fb%3Fc"]);
    expect(play.getAttribute("href")).toBe("/items/a%2Fb%3Fc/play?fileId=f%261%23x");
  });

  it("renders in German with German language names and units", async () => {
    const main = await renderItem(hdrMovie, ["de"]);

    expect(within(main).getByText("1 Std. 30 Min. · 4K")).toBeTruthy();
    expect(within(main).getByRole("link", { name: "Abspielen" })).toBeTruthy();
    const audio = within(main).getByRole("region", { name: "Audio" });
    expect(listItems(audio, "Audio")[0]).toBe("DeutschAC-3 5.1Standard");
    expect(within(main).getByText("276,1 MB")).toBeTruthy();
  });

  it.each([
    { durationSeconds: 90, expected: "1 min" },
    { durationSeconds: 10.2, expected: "10 s" },
  ])(
    "shows a duration of $durationSeconds s as $expected",
    async ({ durationSeconds, expected }) => {
      const main = await renderItem(itemWithDuration(durationSeconds));

      expect(within(main).getByText(expected)).toBeTruthy();
    },
  );

  it("leaves focus where it is on the first load", async () => {
    await renderItem(hdrMovie);

    expect(document.activeElement).toBe(document.body);
  });
});

describe("ItemPage with an item that cannot be played", () => {
  it("marks Play disabled but focusable and explains why", async () => {
    const { router } = renderItemPage("/items/item-2", { fetch: itemServer([unreadableItem]) });
    const main = await findMain();

    const play = await within(main).findByRole("button", {
      name: "Play",
      description: "This file cannot be played because the server could not read it.",
    });
    expect(play.getAttribute("aria-disabled")).toBe("true");
    expect(play).toHaveProperty("disabled", false);
    play.focus();
    expect(document.activeElement).toBe(play);

    fireEvent.click(play);

    expect(router.state.location.pathname).toBe("/items/item-2");
    expect(within(main).queryByRole("link", { name: "Play" })).toBeNull();
  });

  it("shows empty track lists without the subtitle hint and no chips", async () => {
    const main = await renderItem(unreadableItem);

    expect(within(main).queryByRole("list", { name: "Technical details" })).toBeNull();
    expect(within(main).getByText("No audio tracks")).toBeTruthy();
    expect(within(main).getByText("No subtitles")).toBeTruthy();
    expect(within(main).queryByText(subtitleHint)).toBeNull();
    expect(within(main).getByText("512 byte")).toBeTruthy();
  });

  it.each([
    {
      name: "a probe timeout",
      files: [mediaFile({ available: false, errorCode: "probe_timed_out" })],
      explanation: "This file cannot be played because reading it took too long.",
    },
    {
      name: "an unknown error code",
      files: [mediaFile({ available: false, errorCode: "something_new" })],
      explanation: "This file cannot be played.",
    },
    { name: "no file", files: [], explanation: "This item has no file to play." },
  ])("explains $name", async ({ files, explanation }) => {
    const main = await renderItem({ ...unreadableItem, files });

    expect(
      within(main).getByRole("button", { name: "Play", description: explanation }),
    ).toBeTruthy();
  });

  it("shows neither facts nor a file section for an item without files", async () => {
    const main = await renderItem({ ...unreadableItem, files: [] });

    expect(within(main).queryByRole("region", { name: "File" })).toBeNull();
    expect(within(main).queryByText("·", { exact: false })).toBeNull();
  });
});

describe("ItemPage states", () => {
  // That the region mounts empty first is checked in useDeferredAnnouncement.test.ts; here
  // the timer has usually fired before the page is found.
  it("announces loading in a status region while the item is on its way", async () => {
    renderItemPage("/items/item-1", { fetch: () => new Promise<Response>(() => undefined) });

    const main = await findMain();
    const status = within(main).getByRole("status");

    expect(within(main).getByRole("heading", { level: 1, name: "Item details" })).toBeTruthy();
    await waitFor(() => {
      expect(status.textContent).toBe("Loading item details");
    });
  });

  it("shows not found for an unknown item, without retrying, and links home", async () => {
    const server = recordingItemRequests(itemServer([]));
    const { router } = renderItemPage("/items/unknown", { fetch: server.fetch });

    const main = await findMain();
    expect(
      await within(main).findByRole("heading", { level: 1, name: "Item not found" }),
    ).toBeTruthy();
    expect(server.itemRequests).toEqual(["/api/v1/items/unknown"]);

    fireEvent.click(within(main).getByRole("link", { name: "Back to home" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeTruthy();
    expect(router.state.location.pathname).toBe("/");
  });

  it("explains a server error neutrally", async () => {
    const serverError: ItemServerFetch = () =>
      Promise.resolve(
        new Response(JSON.stringify({ ...notFoundProblem, status: 500, code: "internal" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        }),
      );
    renderItemPage("/items/item-1", { fetch: serverError });
    const main = await findMain();

    expect((await within(main).findByRole("alert")).textContent).toBe(serverErrorText);
  });

  it("retries after a network error, then shows the item and focuses its heading", async () => {
    let online = false;
    const reachable = itemServer([hdrMovie]);
    const server = recordingItemRequests((request) =>
      online ? reachable(request) : offline(request),
    );
    renderItemPage("/items/item-1", { fetch: server.fetch });

    const main = await findMain();
    expect(
      await within(main).findByRole("heading", { level: 1, name: "Item could not be loaded" }),
    ).toBeTruthy();
    expect(within(main).getByRole("alert").textContent).toBe(networkErrorText);
    // One automatic retry before the error is shown.
    expect(server.itemRequests).toHaveLength(2);

    online = true;
    fireEvent.click(within(main).getByRole("button", { name: "Try again" }));

    const heading = await within(main).findByRole("heading", { level: 1, name: "Big Buck Bunny" });
    await waitFor(() => {
      expect(document.activeElement).toBe(heading);
    });
    expect(server.itemRequests).toHaveLength(3);
  });

  it("focuses the error heading when the retry fails again", async () => {
    renderItemPage("/items/item-1", { fetch: offline });
    const main = await findMain();

    fireEvent.click(await within(main).findByRole("button", { name: "Try again" }));

    await waitFor(() => {
      expect(document.activeElement).toBe(
        within(main).getByRole("heading", { level: 1, name: "Item could not be loaded" }),
      );
    });
  });

  it("shows the loading state while the retry runs", async () => {
    let attempts = 0;
    // The harness has no app shell, so every request is an item request.
    const failTwiceThenHang: ItemServerFetch = (request) => {
      attempts += 1;
      return attempts <= 2 ? offline(request) : new Promise<Response>(() => undefined);
    };
    renderItemPage("/items/item-1", { fetch: failTwiceThenHang });
    const main = await findMain();

    fireEvent.click(await within(main).findByRole("button", { name: "Try again" }));

    const status = await within(main).findByRole("status");
    await waitFor(() => {
      expect(status.textContent).toBe("Loading item details");
    });
    expect(within(main).queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("keeps showing the item when a background refetch fails", async () => {
    let online = true;
    const reachable = itemServer([hdrMovie]);
    const { queryClient } = renderItemPage("/items/item-1", {
      fetch: (request) => (online ? reachable(request) : offline(request)),
    });
    const main = await findMain();
    await within(main).findByRole("heading", { level: 1, name: "Big Buck Bunny" });
    online = false;

    await queryClient.refetchQueries();
    await waitFor(() => {
      expect(queryClient.getQueryState(itemQueryKey("item-1"))?.status).toBe("error");
    });

    expect(within(main).getByRole("heading", { level: 1, name: "Big Buck Bunny" })).toBeTruthy();
    expect(within(main).queryByRole("alert")).toBeNull();
  });
});

describe("ItemPage in the app", () => {
  it("receives the item ID from the route /items/$itemId inside the app layout", async () => {
    const server = createFakeServer([
      {
        method: "GET",
        path: "/api/v1/items/:itemId",
        respond: ({ params }) =>
          params["itemId"] === hdrMovie.id
            ? jsonResponse(hdrMovie)
            : jsonResponse(notFoundProblem, 404, "application/problem+json"),
      },
    ]);
    renderApp("/items/item-1", { fetch: server.fetch });

    const main = await findMain();

    expect(
      await within(main).findByRole("heading", { level: 1, name: "Big Buck Bunny" }),
    ).toBeTruthy();
    expect(screen.getByRole("navigation", { name: "Main navigation" })).toBeTruthy();
    expect(server.countRequests("GET", "/api/v1/items/item-1")).toBe(1);
  });
});
