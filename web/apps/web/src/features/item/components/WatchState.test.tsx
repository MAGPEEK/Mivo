import type { ItemDetail, ViewerState } from "@mivo/api-client";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { hdrMovie, itemServer, type ItemServerFetch } from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";

const watchedPathPattern = /^\/api\/v1\/items\/([^/]+)\/watched$/;

function state(fields: Partial<ViewerState> = {}): ViewerState {
  return { watched: false, positionSeconds: 0, playCount: 0, ...fields };
}

function movie(viewerState?: ViewerState): ItemDetail {
  return viewerState === undefined ? hdrMovie : { ...hdrMovie, viewerState };
}

type WatchedAnswer = "store" | number | Promise<Response>;

interface WatchServer {
  readonly fetch: ItemServerFetch;
  /** The watch operations the server received, e.g. `PUT item-1`. */
  readonly operations: string[];
}

/**
 * The stub server of these tests (the real one answers 501 until M7-23): it knows `items`,
 * and answers the watch operations by storing the state, with a status or with `answer`.
 */
function watchServer(items: readonly ItemDetail[], answer: WatchedAnswer = "store"): WatchServer {
  const stored = new Map(items.map((item) => [item.id, item]));
  const operations: string[] = [];
  const store = (itemId: string, watched: boolean): Response => {
    const item = stored.get(itemId);
    if (item !== undefined) {
      stored.set(itemId, { ...item, viewerState: state({ watched, playCount: watched ? 1 : 0 }) });
    }
    return new Response(null, { status: 204 });
  };
  const fetch: ItemServerFetch = (request) => {
    const itemId = watchedPathPattern.exec(new URL(request.url).pathname)?.[1];
    if (itemId === undefined) {
      return itemServer([...stored.values()])(request);
    }
    operations.push(`${request.method} ${itemId}`);
    if (answer === "store") {
      return Promise.resolve(store(itemId, request.method === "PUT"));
    }
    return typeof answer === "number"
      ? Promise.resolve(new Response(null, { status: answer }))
      : answer;
  };
  return { fetch, operations };
}

describe("Resume (DT-05)", () => {
  it.each([
    [2530.5, "Resume from 42:10", "2530"],
    [3730, "Resume from 1:02:10", "3730"],
  ])("offers to resume at %d seconds", async (positionSeconds, name, start) => {
    const server = watchServer([movie(state({ positionSeconds }))]);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    const resume = await screen.findByRole("link", { name });
    const search = new URL(resume.getAttribute("href") ?? "", "http://app.test").searchParams;
    expect(search.get("t")).toBe(start);
    expect(search.get("fileId")).toBe("file-1");
    expect(screen.queryByRole("link", { name: "Play" })).toBeNull();
  });

  it("offers Play from start in the split menu, which opens the player without a position", async () => {
    const server = watchServer([movie(state({ positionSeconds: 3730 }))]);
    const { router } = renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "More ways to play" }));
    expect(await blockingAxeViolations(document.body)).toEqual([]);
    fireEvent.click(screen.getByRole("menuitem", { name: "Play from start" }));

    expect(await screen.findByRole("heading", { name: "Player" })).toBeTruthy();
    expect(router.state.location.pathname).toBe("/items/item-1/play");
    expect(router.state.location.search).toEqual({ fileId: "file-1" });
  });

  it.each([
    ["no viewer state", undefined],
    ["no position", state({ watched: true, playCount: 1 })],
    ["less than a second", state({ positionSeconds: 0.4 })],
  ])("offers plain Play with %s", async (_name, viewerState) => {
    const server = watchServer([movie(viewerState)]);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    const play = await screen.findByRole("link", { name: "Play" });
    expect(play.getAttribute("href")).not.toContain("t=");
    expect(screen.queryByRole("button", { name: "More ways to play" })).toBeNull();
  });

  it("is German for a German browser", async () => {
    const server = watchServer([movie(state({ positionSeconds: 3730 }))]);
    renderItemPage("/items/item-1", { fetch: server.fetch, languages: ["de"] });

    expect(await screen.findByRole("link", { name: "Fortsetzen ab 1:02:10" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Weitere Wiedergabeoptionen" }));
    expect(screen.getByRole("menuitem", { name: "Von Anfang an abspielen" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Als gesehen markieren" })).toBeTruthy();
  });
});

describe("Mark as watched on the detail page", () => {
  it("marks a movie watched: the label and Resume change at once and stay", async () => {
    let send: (response: Response) => void = () => undefined;
    const answer = new Promise<Response>((resolve) => {
      send = resolve;
    });
    const server = watchServer([movie(state({ positionSeconds: 3730 }))], answer);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Mark as watched" }));

    const button = await screen.findByRole("button", { name: "Mark as unwatched" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("link", { name: "Play" })).toBeTruthy();
    expect(server.operations).toEqual(["PUT item-1"]);

    send(new Response(null, { status: 204 }));
    await waitFor(() => {
      expect(button.getAttribute("aria-disabled")).toBe("false");
    });
  });

  it("shows the state the server stored after the reload, and marks unwatched again", async () => {
    const server = watchServer([movie()]);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Mark as watched" }));
    const unmark = await screen.findByRole("button", { name: "Mark as unwatched" });
    await waitFor(() => {
      expect(unmark.getAttribute("aria-disabled")).toBe("false");
    });
    fireEvent.click(unmark);

    expect(await screen.findByRole("button", { name: "Mark as watched" })).toBeTruthy();
    expect(server.operations).toEqual(["PUT item-1", "DELETE item-1"]);
  });

  it("rolls back when the request fails and says so", async () => {
    const server = watchServer([movie(state({ positionSeconds: 3730 }))], 500);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Mark as watched" }));

    expect(
      await screen.findByText("The watched state could not be saved. Try again."),
    ).toBeTruthy();
    const button = screen.getByRole("button", { name: "Mark as watched" });
    expect(button.getAttribute("aria-disabled")).toBe("false");
    expect(screen.getByRole("link", { name: "Resume from 1:02:10" })).toBeTruthy();
  });

  it("is disabled with the reason once the server answers 501", async () => {
    const server = watchServer([movie()], 501);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Mark as watched" }));

    const reason = await screen.findByText("This server cannot store what you have watched yet.");
    const button = screen.getByRole("button", { name: "Mark as watched" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(button.getAttribute("aria-describedby")).toBe(reason.id);
    fireEvent.click(button);
    expect(server.operations).toEqual(["PUT item-1"]);

    fireEvent.click(screen.getByRole("button", { name: "More actions" }));
    const entry = screen.getByRole<HTMLButtonElement>("menuitem", { name: "Mark as watched" });
    expect(entry.disabled).toBe(true);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("offers the action in the overflow menu", async () => {
    const server = watchServer([movie()]);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "More actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Mark as watched" }));

    expect(await screen.findByRole("button", { name: "Mark as unwatched" })).toBeTruthy();
    expect(server.operations).toEqual(["PUT item-1"]);
  });

  it.each(["episode", "show", "season"] as const)("offers the action on a %s", async (kind) => {
    const server = watchServer([{ ...hdrMovie, kind, viewerState: state({ watched: true }) }]);
    renderItemPage("/items/item-1", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Mark as unwatched" }));

    await waitFor(() => {
      expect(server.operations).toEqual(["DELETE item-1"]);
    });
  });

  it.each(["extra", "album", "audiobook", "photo"] as const)(
    "has no watch controls on a %s",
    async (kind) => {
      const server = watchServer([{ ...hdrMovie, kind }]);
      renderItemPage("/items/item-1", { fetch: server.fetch });

      await screen.findByRole("heading", { level: 1, name: "Big Buck Bunny" });
      expect(screen.queryByRole("button", { name: /watched/i })).toBeNull();
      expect(screen.queryByRole("menuitem", { name: /watched/i })).toBeNull();
    },
  );
});
