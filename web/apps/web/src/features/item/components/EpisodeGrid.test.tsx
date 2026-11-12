import type { ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  episodeSummary,
  hierarchyServer,
  seasonOneDetail,
  seasonSummary,
  showDetail,
  specialsDetail,
  type HierarchyData,
} from "../test/fakeHierarchyServer";
import { renderItemPage } from "../test/renderItemPage";

function episodes(count: number): ItemSummary[] {
  return Array.from({ length: count }, (_, index) => episodeSummary(index + 1));
}

const data: HierarchyData = {
  details: [showDetail, seasonOneDetail, specialsDetail],
  children: {
    "show-1": [seasonSummary(1, 9), seasonSummary(0, 3)],
    "season-1": [
      episodeSummary(7, { media: { available: true, durationSeconds: 2700 } }),
      ...episodes(2),
    ],
    "season-0": episodes(1),
  },
};

async function renderAt(path: string) {
  const server = hierarchyServer(data);
  const rendered = renderItemPage(path, { fetch: server.fetch, languages: ["en"] });
  const main = await screen.findByRole("main");
  await within(main).findByRole("heading", { level: 1 });
  return { ...rendered, main };
}

beforeEach(() => {
  window.localStorage.clear();
});

describe("seasons row", () => {
  it("shows the seasons as a scrolling row with the specials last", async () => {
    const { main } = await renderAt("/items/show-1");

    // The row replaces the loading section, so it is looked up after the cards arrived.
    await within(main).findByRole("link", { name: "Specials · 3 episodes" });
    const row = within(main).getByRole("region", { name: "Seasons" });
    const names = within(row)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(names).toEqual(["Season 1 · 9 episodes", "Specials · 3 episodes"]);
    expect(row.querySelector("[data-row-scroller]")).not.toBeNull();
  });
});

describe("episode view controls", () => {
  it("offers the view switch and the size slider on a season", async () => {
    const { main } = await renderAt("/items/season-1");

    expect(await within(main).findByRole("radio", { name: "Grid" })).toBeTruthy();
    expect(within(main).getByRole("radio", { name: "List" })).toBeTruthy();
    expect(within(main).getByRole("slider", { name: "Card size" })).toBeTruthy();
  });

  it("sizes the grid columns with the wide episode format and the slider step", async () => {
    const { main } = await renderAt("/items/season-1");
    const region = await within(main).findByRole("region", { name: "Episodes" });
    await within(region).findByRole("link", { name: "Episode title 7" });
    const grid = within(region).getAllByRole("list")[0];

    // Default step 2 is 160 px, wide cards get 1.6 times that.
    expect(grid?.style.gridTemplateColumns).toBe("repeat(auto-fill, minmax(256px, 1fr))");

    fireEvent.change(within(main).getByRole("slider", { name: "Card size" }), {
      target: { value: "4" },
    });

    await waitFor(() => {
      expect(grid?.style.gridTemplateColumns).toBe("repeat(auto-fill, minmax(368px, 1fr))");
    });
  });

  it("shows title, number and runtime on a grid card", async () => {
    const { main } = await renderAt("/items/season-1");
    const region = await within(main).findByRole("region", { name: "Episodes" });

    await waitFor(() => {
      expect(within(region).getAllByRole("listitem")[0]?.textContent).toBe(
        "Episode title 7Episode 7 · 45 min",
      );
    });
  });

  it("switches to the list, hides the slider and keeps the choice per device", async () => {
    const { main } = await renderAt("/items/season-1");
    await within(main).findByRole("link", { name: "Episode title 7" });

    fireEvent.click(within(main).getByRole("radio", { name: "List" }));

    await waitFor(() => {
      expect(within(main).queryByRole("slider", { name: "Card size" })).toBeNull();
    });
    const region = within(main).getByRole("region", { name: "Episodes" });
    expect(region.querySelector("[data-poster-placeholder]")).toBeNull();
    expect(within(region).getAllByRole("listitem")[0]?.textContent).toContain("Episode title 7");
    expect(window.localStorage.length).toBeGreaterThan(0);
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("keeps the order link next to the view controls", async () => {
    const { main } = await renderAt("/items/season-1");

    expect(await within(main).findByRole("link", { name: "Show last first" })).toBeTruthy();
  });
});
