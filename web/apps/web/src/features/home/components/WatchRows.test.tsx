import type { HomeRows, Row } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { fakeItem, librariesRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { moviesLibrary, showsLibrary } from "../test/homeFixtures";

const episode = fakeItem({
  id: "episode-1",
  kind: "episode",
  title: "Pilot Run",
  showTitle: "Cosmos Laundromat",
  seasonNumber: 3,
  number: 9,
  media: { available: true, durationSeconds: 1000 },
  viewerState: { watched: false, positionSeconds: 500, playCount: 0 },
});
const movie = fakeItem({
  id: "movie-1",
  title: "Sintel",
  media: { available: true, durationSeconds: 888 },
  viewerState: { watched: false, positionSeconds: 222, playCount: 0 },
});
const nextEpisode = fakeItem({
  id: "episode-2",
  kind: "episode",
  title: "Second Run",
  showTitle: "Elephants Dream",
  seasonNumber: 1,
  number: 2,
});

function row(kind: "continueWatching" | "upNext", items: Row["items"]): Row {
  return {
    id: kind,
    kind,
    titleKey: `rows.${kind}`,
    cardFormat: "poster",
    items,
  };
}

/** A home rows route whose rows follow the requests the test makes. */
function stateful(rows: () => Row[]): FakeRoute {
  return {
    method: "GET",
    path: "/api/v1/home/rows",
    respond: () => jsonResponse({ rows: rows(), hero: [] } satisfies HomeRows),
  };
}

function renderHome(rows: () => Row[], actions: readonly FakeRoute[] = []) {
  const server = createFakeServer([
    librariesRoute([moviesLibrary, showsLibrary]),
    stateful(rows),
    ...actions,
  ]);
  renderApp("/", { fetch: server.fetch });
  return server;
}

const noContent = () => new Response(null, { status: 204 });

describe("Continue watching and Up next rows", () => {
  it("renders both kinds with progress, show title and the compact episode label", async () => {
    renderHome(() => [row("continueWatching", [episode, movie]), row("upNext", [nextEpisode])]);

    const continueRow = await screen.findByRole("region", { name: "Continue watching" });
    const link = within(continueRow).getByRole("link", { name: "Cosmos Laundromat" });
    expect(link.getAttribute("href")).toBe("/items/episode-1/play?t=500");
    expect(within(continueRow).getByText("S3 · E9")).toBeTruthy();
    expect(within(continueRow).getByText("Pilot Run")).toBeTruthy();
    expect(
      within(continueRow)
        .getAllByRole("progressbar")
        .map((bar) => bar.getAttribute("aria-valuenow")),
    ).toEqual(["50", "25"]);
    expect(within(continueRow).getByRole("link", { name: "Sintel" }).getAttribute("href")).toBe(
      "/items/movie-1/play?t=222",
    );
    const upNext = await screen.findByRole("region", { name: "Up next" });
    expect(within(upNext).getByRole("link", { name: "Elephants Dream" }).getAttribute("href")).toBe(
      "/items/episode-2",
    );
    expect(within(upNext).getByText("S1 · E2")).toBeTruthy();
    expect(within(upNext).queryByRole("button", { name: /More actions/ })).toBeNull();
  });

  it("titles the rows in German", async () => {
    const server = createFakeServer([
      librariesRoute([moviesLibrary]),
      stateful(() => [row("continueWatching", [movie]), row("upNext", [nextEpisode])]),
    ]);
    renderApp("/", { languages: ["de"], fetch: server.fetch });

    expect(await screen.findByRole("region", { name: "Weiterschauen" })).toBeTruthy();
    expect(await screen.findByRole("region", { name: "Als Nächstes" })).toBeTruthy();
  });

  it("shows no Continue watching row when the server sends none", async () => {
    renderHome(() => [row("upNext", [nextEpisode])]);

    await screen.findByRole("region", { name: "Up next" });
    expect(screen.queryByRole("region", { name: "Continue watching" })).toBeNull();
  });

  it("removes an entry and reloads the rows", async () => {
    let rows = [row("continueWatching", [episode, movie])];
    const server = renderHome(
      () => rows,
      [
        {
          method: "DELETE",
          path: "/api/v1/items/:itemId/continue-watching",
          respond: () => {
            rows = [row("continueWatching", [movie])];
            return noContent();
          },
        },
      ],
    );
    const region = await screen.findByRole("region", { name: "Continue watching" });

    fireEvent.click(
      within(region).getByRole("button", { name: "More actions for Cosmos Laundromat" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove from continue watching" }));

    await waitFor(() => {
      expect(within(region).queryByRole("link", { name: "Cosmos Laundromat" })).toBeNull();
    });
    expect(server.countRequests("DELETE", "/api/v1/items/episode-1/continue-watching")).toBe(1);
    expect(server.countRequests("GET", "/api/v1/home/rows")).toBe(2);
    // Focus moved to the next card before the card left, and the result was announced.
    expect(document.activeElement).toBe(within(region).getByRole("link", { name: "Sintel" }));
    expect(await screen.findByText("Removed from continue watching.")).toBeTruthy();
  });

  it("marks an entry watched and reloads the rows", async () => {
    let rows = [row("continueWatching", [movie])];
    const server = renderHome(
      () => rows,
      [
        {
          method: "PUT",
          path: "/api/v1/items/:itemId/watched",
          respond: () => {
            rows = [];
            return noContent();
          },
        },
      ],
    );
    const region = await screen.findByRole("region", { name: "Continue watching" });

    fireEvent.click(within(region).getByRole("button", { name: "More actions for Sintel" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Mark as watched" }));

    await waitFor(() => {
      expect(screen.queryByRole("region", { name: "Continue watching" })).toBeNull();
    });
    expect(server.countRequests("PUT", "/api/v1/items/movie-1/watched")).toBe(1);
  });

  it("reports a server without watch state as a failed action", async () => {
    renderHome(
      () => [row("continueWatching", [movie])],
      [
        {
          method: "DELETE",
          path: "/api/v1/items/:itemId/continue-watching",
          respond: () => problemResponse(501, "not_implemented"),
        },
      ],
    );
    const region = await screen.findByRole("region", { name: "Continue watching" });

    fireEvent.click(within(region).getByRole("button", { name: "More actions for Sintel" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove from continue watching" }));

    expect(await within(region).findByText("The action could not be completed.")).toBeTruthy();
  });

  it("announces a failed action and keeps the card", async () => {
    renderHome(
      () => [row("continueWatching", [movie])],
      [
        {
          method: "DELETE",
          path: "/api/v1/items/:itemId/continue-watching",
          respond: () => problemResponse(500, "internal"),
        },
      ],
    );
    const region = await screen.findByRole("region", { name: "Continue watching" });

    fireEvent.click(within(region).getByRole("button", { name: "More actions for Sintel" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove from continue watching" }));

    expect(await within(region).findByText("The action could not be completed.")).toBeTruthy();
    expect(within(region).getByRole("link", { name: "Sintel" })).toBeTruthy();
    expect(await blockingAxeViolations(region)).toEqual([]);
  });
});
