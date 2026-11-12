import type { ExtraType, ItemDetail, ItemSummary } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { hdrMovie, itemServer } from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";

function extra(id: string, title: string, extraType: ExtraType): ItemSummary {
  return { id, libraryId: "library-1", kind: "extra", title, extraType };
}

const movieWithCredits: ItemDetail = {
  ...hdrMovie,
  people: [
    { name: "Ines Carvalho", role: "actor", character: "Ines" },
    { name: "Mara Lindqvist", role: "director" },
  ],
  extras: [
    extra("extra-1", "Official Teaser", "trailer"),
    extra("extra-2", "Making-of", "featurette"),
    extra("extra-3", "The Cellar", "deleted_scene"),
  ],
};

async function renderItem(item: ItemDetail, languages: readonly string[] = ["en"]) {
  renderItemPage(`/items/${encodeURIComponent(item.id)}`, {
    fetch: itemServer([item]),
    languages,
  });
  const main = await screen.findByRole("main");
  await within(main).findByRole("heading", { level: 1, name: item.title });
  return main;
}

describe("cast and crew row", () => {
  it("lists names with character or role and initials instead of portraits", async () => {
    const main = await renderItem(movieWithCredits);

    const cast = within(main).getByRole("region", { name: "Cast & crew" });
    expect(within(cast).getByText("Ines Carvalho")).toBeTruthy();
    expect(within(cast).getByText("Ines")).toBeTruthy();
    expect(within(cast).getByText("Director")).toBeTruthy();
    expect(within(cast).getByText("IC").getAttribute("aria-hidden")).toBe("true");
    expect(within(cast).queryByRole("link")).toBeNull();
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("is hidden without people", async () => {
    const plain = await renderItem(hdrMovie);
    expect(within(plain).queryByRole("region", { name: "Cast & crew" })).toBeNull();
  });

  it("is translated to German", async () => {
    const main = await renderItem(movieWithCredits, ["de"]);

    const cast = within(main).getByRole("region", { name: "Besetzung & Crew" });
    expect(within(cast).getByText("Regie")).toBeTruthy();
  });
});

describe("extras rows", () => {
  it("groups the extras by kind and links each card to its page", async () => {
    const main = await renderItem(movieWithCredits);

    const trailers = within(main).getByRole("region", { name: "Trailers" });
    expect(
      within(trailers).getByRole("link", { name: "Official Teaser" }).getAttribute("href"),
    ).toBe("/items/extra-1");
    expect(within(main).getByRole("region", { name: "Featurettes" })).toBeTruthy();
    expect(within(main).getByRole("region", { name: "Deleted scenes" })).toBeTruthy();
    expect(within(main).queryByRole("region", { name: "Behind the scenes" })).toBeNull();
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("shows no extras row without extras", async () => {
    const main = await renderItem(hdrMovie);

    expect(within(main).queryByRole("region", { name: "Trailers" })).toBeNull();
  });
});

describe("trailer button", () => {
  it("plays the first trailer through the player route", async () => {
    const main = await renderItem(movieWithCredits);

    const button = within(main).getByRole("link", { name: "Trailer" });
    expect(button.getAttribute("href")).toBe("/items/extra-1/play");
    fireEvent.click(button);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Player" })).toBeTruthy();
    });
  });

  it("is hidden without a trailer", async () => {
    const main = await renderItem({
      ...movieWithCredits,
      extras: [extra("extra-2", "Making-of", "featurette")],
    });

    expect(within(main).queryByRole("link", { name: "Trailer" })).toBeNull();
  });
});
