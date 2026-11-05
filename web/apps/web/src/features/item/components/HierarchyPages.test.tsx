import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  episodeDetail,
  episodeSummary,
  hierarchyServer,
  seasonOneDetail,
  seasonSummary,
  showDetail,
  showWithoutPlayDetail,
  specialsDetail,
  type HierarchyData,
} from "../test/fakeHierarchyServer";
import { renderItemPage } from "../test/renderItemPage";

const seasonSummaries = [seasonSummary(1, 9), seasonSummary(0, 3)];

function episodes(count: number): ItemSummary[] {
  return Array.from({ length: count }, (_, index) => episodeSummary(index + 1));
}

const baseData: HierarchyData = {
  details: [showDetail, seasonOneDetail, specialsDetail],
  children: { "show-1": seasonSummaries, "season-1": episodes(9), "season-0": episodes(3) },
};

async function renderAt(
  path: string,
  data: HierarchyData = baseData,
  languages: readonly string[] = ["en"],
) {
  const server = hierarchyServer(data);
  const rendered = renderItemPage(path, { fetch: server.fetch, languages });
  const main = await screen.findByRole("main");
  await within(main).findByRole("heading", { level: 1 });
  return { ...rendered, main, server };
}

function itemNames(container: HTMLElement, name: string): string[] {
  return within(within(container).getByRole("region", { name }))
    .getAllByRole("listitem")
    .map((element) => element.textContent);
}

describe("show page", () => {
  it("shows poster, title, year, summary, Play and the seasons with the specials last", async () => {
    const { main } = await renderAt("/items/show-1");

    expect(within(main).getByRole("heading", { level: 1, name: "The Test Show" })).toBeTruthy();
    expect(within(main).getByText("2008 · 2 seasons · 12 episodes")).toBeTruthy();
    expect(within(main).getByText("A chemistry teacher tests things.")).toBeTruthy();
    expect(within(main).getByRole("list", { name: "Genres" }).textContent).toBe("Drama");
    expect(main.querySelector("img")?.getAttribute("alt")).toBe("");
    expect(within(main).getByRole("link", { name: "Play" }).getAttribute("href")).toBe(
      "/items/episode-1/play",
    );
    await waitFor(() => {
      expect(itemNames(main, "Seasons")).toEqual([
        "Season 1 · 9 episodes",
        "Specials · 3 episodes",
      ]);
    });
    expect(
      within(main).getByRole("link", { name: "Season 1 · 9 episodes" }).getAttribute("href"),
    ).toBe("/items/season-1");
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("disables Play with a hint when there is nothing to play", async () => {
    const { main } = await renderAt("/items/show-1", {
      ...baseData,
      details: [showWithoutPlayDetail, seasonOneDetail],
    });

    const play = within(main).getByRole("button", { name: "Play" });
    expect(play.getAttribute("aria-disabled")).toBe("true");
    expect(within(main).getByText("There is no episode to play yet.")).toBeTruthy();
  });

  it("lists a show with only specials", async () => {
    const { main } = await renderAt("/items/show-1", {
      ...baseData,
      children: { ...baseData.children, "show-1": [seasonSummary(0, 3)] },
    });

    await waitFor(() => {
      expect(itemNames(main, "Seasons")).toEqual(["Specials · 3 episodes"]);
    });
  });

  it("counts seasons with the plural forms of the language", async () => {
    const { main } = await renderAt("/items/show-1", baseData, ["de"]);

    expect(within(main).getByText("2008 · 2 Staffeln · 12 Folgen")).toBeTruthy();
    await waitFor(() => {
      expect(itemNames(main, "Staffeln")).toEqual(["Staffel 1 · 9 Folgen", "Specials · 3 Folgen"]);
    });
  });

  it("shows a placeholder for a season without a poster", async () => {
    const { main } = await renderAt("/items/show-1");

    await within(main).findByRole("link", { name: "Specials · 3 episodes" });
    expect(main.querySelectorAll("[data-poster-placeholder]").length).toBeGreaterThan(0);
  });
});

describe("season page", () => {
  it("links to the show and lists the episodes in order with label and runtime", async () => {
    const children = [
      episodeSummary(7, { media: { available: true, durationSeconds: 2700 } }),
      episodeSummary(8),
      episodeSummary(undefined, { id: "episode-bonus", title: "Behind the scenes" }),
    ];
    const { main } = await renderAt("/items/season-1", {
      ...baseData,
      children: { ...baseData.children, "season-1": children },
    });

    expect(within(main).getByRole("link", { name: "The Test Show" }).getAttribute("href")).toBe(
      "/items/show-1",
    );
    expect(within(main).getByText("9 episodes")).toBeTruthy();
    await waitFor(() => {
      expect(itemNames(main, "Episodes")).toEqual([
        "Episode title 7Episode 7 · 45 min",
        "Episode title 8Episode 8",
        "Behind the scenes",
      ]);
    });
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("hides the arrow at the ends and names the other one", async () => {
    const first = await renderAt("/items/season-1");

    expect(within(first.main).queryByRole("link", { name: /^Previous season/ })).toBeNull();
    await waitFor(() => {
      expect(within(first.main).getByRole("link", { name: "Next season: Specials" })).toBeTruthy();
    });
  });

  it("has no next arrow on the last season", async () => {
    const { main } = await renderAt("/items/season-0");

    expect(within(main).queryByRole("link", { name: /^Next season/ })).toBeNull();
    await waitFor(() => {
      expect(within(main).getByRole("link", { name: "Previous season: Season 1" })).toBeTruthy();
    });
  });

  it("moves focus to the heading after following an arrow", async () => {
    const { main } = await renderAt("/items/season-1");

    fireEvent.click(await within(main).findByRole("link", { name: "Next season: Specials" }));

    const heading = await within(main).findByRole("heading", { level: 1, name: "Specials" });
    await waitFor(() => {
      expect(document.activeElement).toBe(heading);
    });
  });

  it("toggles the order through the URL and back", async () => {
    const { main, router, server } = await renderAt("/items/season-1");
    await waitFor(() => {
      expect(itemNames(main, "Episodes")[0]).toContain("Episode title 1");
    });

    fireEvent.click(within(main).getByRole("link", { name: "Show last first" }));

    await waitFor(() => {
      expect(itemNames(main, "Episodes")[0]).toContain("Episode title 9");
    });
    expect(router.state.location.search).toEqual({ order: "desc" });
    expect(
      server.requests.some((request) => new URL(request.url).searchParams.get("order") === "desc"),
    ).toBe(true);

    fireEvent.click(within(main).getByRole("link", { name: "Show first first" }));

    await waitFor(() => {
      expect(itemNames(main, "Episodes")[0]).toContain("Episode title 1");
    });
    expect(router.state.location.search).toEqual({});
  });

  it("opens descending from a shared link", async () => {
    const { main } = await renderAt("/items/season-1?order=desc");

    await waitFor(() => {
      expect(itemNames(main, "Episodes")[0]).toContain("Episode title 9");
    });
    expect(within(main).getByRole("link", { name: "Show first first" })).toBeTruthy();
  });

  it("shows the first page of 131 episodes and loads more", async () => {
    const { main } = await renderAt("/items/season-1", {
      ...baseData,
      children: { ...baseData.children, "season-1": episodes(131) },
    });
    await waitFor(() => {
      expect(itemNames(main, "Episodes")).toHaveLength(60);
    });

    fireEvent.click(within(main).getByRole("button", { name: "Load more" }));
    await waitFor(() => {
      expect(itemNames(main, "Episodes")).toHaveLength(120);
    });
    fireEvent.click(within(main).getByRole("button", { name: "Load more" }));

    await waitFor(() => {
      expect(itemNames(main, "Episodes")).toHaveLength(131);
    });
    expect(within(main).queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("shows a placeholder still for an episode without one", async () => {
    const { main } = await renderAt("/items/season-1");

    await within(main).findByRole("link", { name: "Episode title 1" });
    expect(main.querySelectorAll("[data-poster-placeholder]").length).toBeGreaterThan(0);
  });
});

function episodeData(detail: ItemDetail, summary: ItemSummary): HierarchyData {
  return {
    details: [...baseData.details, detail, episodeDetail("episode-2", { title: "Next one" })],
    children: { ...baseData.children, "season-1": [summary, episodeSummary(2)] },
  };
}

describe("episode page", () => {
  it("shows the code, air date, links, arrows and the technical details", async () => {
    const detail = episodeDetail("episode-3", {
      number: 3,
      releaseDate: "2008-01-20",
      previousId: "episode-2",
      nextId: "episode-4",
      artwork: { thumb: { id: "still-1", tag: "tag-1" } },
    });
    const { main } = await renderAt("/items/episode-3", {
      details: [...baseData.details, detail, episodeDetail("episode-2", { title: "Before" })],
      children: { ...baseData.children, "season-1": [episodeSummary(3)] },
    });

    expect(await within(main).findByText("S1 · E3 · Aired January 20, 2008 · 45 min")).toBeTruthy();
    expect(within(main).getByRole("link", { name: "The Test Show" }).getAttribute("href")).toBe(
      "/items/show-1",
    );
    expect(within(main).getByRole("link", { name: "Season 1" }).getAttribute("href")).toBe(
      "/items/season-1",
    );
    expect(main.querySelector("img")?.getAttribute("src")).toContain("still-1");
    expect(within(main).getByRole("list", { name: "Technical details" }).textContent).toBe("MKV");
    expect(within(main).getByRole("link", { name: "Play" }).getAttribute("href")).toContain(
      "/items/episode-3/play?fileId=episode-file",
    );
    await waitFor(() => {
      expect(within(main).getByRole("link", { name: "Previous episode: Before" })).toBeTruthy();
    });
    expect(within(main).getByRole("link", { name: /^Next episode/ })).toBeTruthy();
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("labels a multi-episode file with its range", async () => {
    const detail = episodeDetail("episode-1", { number: 1, numberEnd: 2 });
    const { main } = await renderAt("/items/episode-1", episodeData(detail, episodeSummary(1)));

    expect(await within(main).findByText("S1 · E1–E2 · 45 min")).toBeTruthy();
  });

  it("shows a placeholder still and no code or arrows without that data", async () => {
    const detail = episodeDetail("episode-9", { ancestors: [] });
    const { main } = await renderAt("/items/episode-9", { ...baseData, details: [detail] });

    expect(main.querySelector("[data-poster-placeholder]")).not.toBeNull();
    expect(within(main).queryByRole("navigation")).toBeNull();
    expect(within(main).queryByText(/^S\d/)).toBeNull();
  });
});

describe("movie page", () => {
  it("shows year, genres and summary when present", async () => {
    const movie: ItemDetail = {
      id: "movie-1",
      libraryId: "library-1",
      kind: "movie",
      title: "Big Buck Bunny",
      year: 2008,
      genres: ["Animation", "Comedy"],
      summary: "A giant rabbit takes revenge.",
      files: [],
    };
    const { main } = await renderAt("/items/movie-1", { details: [movie], children: {} });

    expect(within(main).getByText("2008")).toBeTruthy();
    expect(within(main).getByRole("list", { name: "Genres" }).textContent).toBe("AnimationComedy");
    expect(within(main).getByText("A giant rabbit takes revenge.")).toBeTruthy();
  });
});
