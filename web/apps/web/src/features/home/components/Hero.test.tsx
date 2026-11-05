import type { HomeRows, ItemDetail } from "@mivo/api-client";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, jsonResponse, type FakeRoute } from "../../../test/fakeApi";
import { librariesRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";
import { heroRotationIntervalMs } from "../hooks/useHeroRotation";
import { heroEnabledStorageKey } from "../lib/heroPreference";
import {
  heroShow,
  homeRowsRoute,
  moviesLibrary,
  rowsWithHero,
  showsLibrary,
  twoLibraryRows,
} from "../test/homeFixtures";

const firstEpisodeId = "episode-1";

// A show without episodes names nothing to play.
const emptyShowDetail: ItemDetail = {
  id: heroShow.itemId,
  libraryId: heroShow.libraryId,
  kind: "show",
  title: heroShow.title,
  files: [],
};

const showDetail: ItemDetail = { ...emptyShowDetail, playItemId: firstEpisodeId };

function showRoute(detail: ItemDetail): FakeRoute {
  return {
    method: "GET",
    path: `/api/v1/items/${detail.id}`,
    respond: () => jsonResponse(detail),
  };
}

function renderHome(rows: HomeRows = rowsWithHero, extraRoutes: readonly FakeRoute[] = []) {
  const server = createFakeServer([
    librariesRoute([moviesLibrary, showsLibrary]),
    homeRowsRoute(rows),
    ...extraRoutes,
  ]);
  return renderApp("/", { fetch: server.fetch });
}

async function heroRegion(): Promise<HTMLElement> {
  return screen.findByRole("region", { name: "Featured" });
}

describe("Hero", () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it("shows the newest title with its meta line, summary and actions", async () => {
    renderHome();

    const hero = await heroRegion();
    expect(hero.getAttribute("aria-roledescription")).toBe("carousel");
    expect(within(hero).getByRole("heading", { level: 2, name: "Sintel" })).toBeTruthy();
    expect(within(hero).getByText("2010 · 15 min · Animation, Fantasy · 1080p")).toBeTruthy();
    expect(within(hero).getByText("A lonely girl searches for her dragon.")).toBeTruthy();
    expect(within(hero).getByRole("group", { name: "1 of 2" })).toBeTruthy();
    expect(within(hero).getByRole("link", { name: "Play" }).getAttribute("href")).toBe(
      "/items/movie-1/play",
    );
    expect(within(hero).getByRole("link", { name: "More info" }).getAttribute("href")).toBe(
      "/items/movie-1",
    );
    const backdrop = hero.querySelector("img");
    expect(backdrop?.getAttribute("src")).toContain("/api/v1/images/backdrop-movie-1");
    expect(backdrop?.getAttribute("alt")).toBe("");
  });

  it("names its dots and marks the current one", async () => {
    renderHome();

    const hero = await heroRegion();
    const dots = within(hero).getByRole("group", { name: "Featured titles" });
    const sintel = within(dots).getByRole("button", { name: "Show Sintel" });
    const cosmos = within(dots).getByRole("button", { name: "Show Cosmos Laundromat" });
    expect(sintel.getAttribute("aria-current")).toBe("true");
    expect(cosmos.getAttribute("aria-current")).toBeNull();

    fireEvent.click(cosmos);

    expect(within(hero).getByRole("heading", { level: 2, name: "Cosmos Laundromat" })).toBeTruthy();
    expect(cosmos.getAttribute("aria-current")).toBe("true");
  });

  it("offers a stop control first and keeps a stopped rotation still", async () => {
    renderHome();
    const hero = await heroRegion();
    const toggle = within(hero).getByRole("button", { name: "Pause rotation" });
    expect(within(hero).getAllByRole("button")[0]).toBe(toggle);

    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-label")).toBe("Start rotation");

    vi.useFakeTimers();
    try {
      act(() => {
        vi.advanceTimersByTime(heroRotationIntervalMs * 2);
      });
      expect(within(hero).getByRole("heading", { level: 2, name: "Sintel" })).toBeTruthy();

      fireEvent.click(toggle);
      expect(toggle.getAttribute("aria-label")).toBe("Pause rotation");
      act(() => {
        vi.advanceTimersByTime(heroRotationIntervalMs);
      });
      expect(
        within(hero).getByRole("heading", { level: 2, name: "Cosmos Laundromat" }),
      ).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it("offers no stop control for a single title", async () => {
    renderHome({ ...rowsWithHero, hero: [heroShow] });

    const hero = await heroRegion();
    expect(within(hero).queryByRole("button", { name: "Pause rotation" })).toBeNull();
  });

  it("plays a show from the episode the server names", async () => {
    const { router } = renderHome(rowsWithHero, [showRoute(showDetail)]);
    const hero = await heroRegion();
    fireEvent.click(within(hero).getByRole("button", { name: "Show Cosmos Laundromat" }));

    fireEvent.click(within(hero).getByRole("button", { name: "Play" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/items/${firstEpisodeId}/play`);
    });
  });

  it("opens the show's page when it has nothing to play", async () => {
    const { router } = renderHome(rowsWithHero, [showRoute(emptyShowDetail)]);
    const hero = await heroRegion();
    fireEvent.click(within(hero).getByRole("button", { name: "Show Cosmos Laundromat" }));

    fireEvent.click(within(hero).getByRole("button", { name: "Play" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/items/${heroShow.itemId}`);
    });
  });

  it("is not shown without titles with artwork", async () => {
    renderHome(twoLibraryRows);

    await screen.findByRole("region", { name: "Recently added in Movies" });
    expect(screen.queryByRole("region", { name: "Featured" })).toBeNull();
  });

  it("is not shown when this device switched it off", async () => {
    window.localStorage.setItem(heroEnabledStorageKey, "false");
    renderHome();

    await screen.findByRole("region", { name: "Recently added in Movies" });
    expect(screen.queryByRole("region", { name: "Featured" })).toBeNull();
  });

  it("speaks German", async () => {
    const server = createFakeServer([
      librariesRoute([moviesLibrary, showsLibrary]),
      homeRowsRoute(rowsWithHero),
    ]);
    renderApp("/", { languages: ["de"], fetch: server.fetch });

    const hero = await screen.findByRole("region", { name: "Empfohlen" });
    expect(within(hero).getByText("2010 · 15 Min. · Animation, Fantasy · 1080p")).toBeTruthy();
    expect(within(hero).getByRole("link", { name: "Mehr Infos" })).toBeTruthy();
    expect(within(hero).getByRole("button", { name: "Sintel zeigen" })).toBeTruthy();
  });

  it("has no accessibility violations", async () => {
    renderHome();

    const hero = await heroRegion();
    expect(await blockingAxeViolations(hero)).toEqual([]);
  });
});
