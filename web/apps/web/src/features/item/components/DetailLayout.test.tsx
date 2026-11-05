import type { ItemDetail } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { backdropScrimClass } from "../lib/backdropScrim";
import { itemServer, mediaFile } from "../test/fakeItemServer";
import { renderItemPage, type RenderedItemPage } from "../test/renderItemPage";

const fullMovie: ItemDetail = {
  id: "movie-1",
  libraryId: "library-7",
  kind: "movie",
  title: "The Lighthouse Keeper",
  originalTitle: "Le Gardien du Phare",
  contentRating: "FSK 12",
  year: 2021,
  tagline: "Some lights should never come back on.",
  summary: "After her father's death Ines takes over the lighthouse.",
  genres: ["Drama", "Mystery", "Thriller", "History", "Romance"],
  people: [
    { name: "Mara Lindqvist", role: "director" },
    { name: "Ines Actor", role: "actor" },
  ],
  artwork: {
    poster: { id: "poster-1", tag: "p" },
    backdrop: { id: "backdrop-1", tag: "b" },
  },
  files: [mediaFile({ durationSeconds: 5940, streams: [] })],
};

const bareMovie: ItemDetail = {
  id: "movie-2",
  libraryId: "library-7",
  kind: "movie",
  title: "Summer Party",
  originalTitle: "",
  contentRating: " ",
  summary: "",
  tagline: "",
  genres: [],
  files: [],
};

interface RenderedDetail extends RenderedItemPage {
  readonly main: HTMLElement;
}

async function renderDetail(
  item: ItemDetail,
  languages: readonly string[] = ["en"],
): Promise<RenderedDetail> {
  const rendered = renderItemPage(`/items/${item.id}`, { fetch: itemServer([item]), languages });
  const main = await screen.findByRole("main");
  await within(main).findByRole("heading", { level: 1, name: item.title });
  return { ...rendered, main };
}

function backdropOf(main: HTMLElement): HTMLElement {
  const backdrop = main.querySelector<HTMLElement>("[data-backdrop]");
  if (backdrop === null) {
    throw new Error("no backdrop rendered");
  }
  return backdrop;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("DetailLayout backdrop", () => {
  it("blurs the backdrop artwork under the scrim token, lazily loaded", async () => {
    const { main } = await renderDetail(fullMovie);

    const backdrop = backdropOf(main);
    expect(backdrop.getAttribute("aria-hidden")).toBe("true");
    expect(backdrop.dataset["backdrop"]).toBe("image");
    const image = backdrop.querySelector("img");
    expect(image?.getAttribute("src")).toContain("/api/v1/images/backdrop-1");
    expect(image?.getAttribute("loading")).toBe("lazy");
    expect(image?.getAttribute("alt")).toBe("");
    expect(backdrop.querySelector("[data-backdrop-scrim]")?.className).toContain(
      backdropScrimClass,
    );
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("falls back to the poster without a backdrop", async () => {
    const { main } = await renderDetail({
      ...fullMovie,
      artwork: { poster: { id: "poster-1", tag: "p" } },
    });

    expect(backdropOf(main).querySelector("img")?.getAttribute("src")).toContain(
      "/api/v1/images/poster-1",
    );
  });

  it("shows a plain background without any artwork", async () => {
    const { main } = await renderDetail(bareMovie);

    const backdrop = backdropOf(main);
    expect(backdrop.dataset["backdrop"]).toBe("plain");
    expect(backdrop.querySelector("img")).toBeNull();
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("falls back to the plain background when the image fails to load", async () => {
    const { main } = await renderDetail(fullMovie);

    const image = backdropOf(main).querySelector("img");
    if (image === null) {
      throw new Error("no backdrop image");
    }
    fireEvent.error(image);

    expect(backdropOf(main).dataset["backdrop"]).toBe("plain");
  });
});

describe("DetailLayout title block", () => {
  it("shows original title, rating, year, runtime and director", async () => {
    const { main } = await renderDetail(fullMovie);

    expect(within(main).getByText("Original title: Le Gardien du Phare")).toBeTruthy();
    expect(within(main).getByText("2021 · 1 h 39 min")).toBeTruthy();
    expect(within(main).getByText("Director: Mara Lindqvist")).toBeTruthy();
    expect(within(main).getByText("Content rating:", { exact: false }).textContent).toBe(
      "Content rating: ",
    );
    expect(main.textContent).toContain("FSK 12");
  });

  it("localises the runtime and labels in German", async () => {
    const { main } = await renderDetail(fullMovie, ["de"]);

    expect(within(main).getByText("2021 · 1 Std. 39 Min.")).toBeTruthy();
    expect(within(main).getByText("Originaltitel: Le Gardien du Phare")).toBeTruthy();
    expect(within(main).getByText("Regie: Mara Lindqvist")).toBeTruthy();
    expect(within(main).getByRole("button", { name: "+2 weitere Genres" })).toBeTruthy();
  });

  it("shows three genres and opens the rest with +n", async () => {
    const { main } = await renderDetail(fullMovie);

    const genres = within(main).getByRole("list", { name: "Genres" });
    expect(
      within(genres)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Drama", "Mystery", "Thriller"]);
    const more = within(main).getByRole("button", { name: "+2 more genres" });
    expect(more.textContent).toBe("+2");
    expect(more.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(more);

    expect(within(genres).getAllByRole("link")).toHaveLength(5);
    const fewer = within(main).getByRole("button", { name: "Fewer genres" });
    expect(fewer.getAttribute("aria-expanded")).toBe("true");
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("links a genre to the library's Browse pivot filtered by it", async () => {
    const { main, router } = await renderDetail(fullMovie);

    fireEvent.click(within(main).getByRole("link", { name: "Mystery" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/libraries/library-7");
    });
    expect(router.state.location.search).toEqual({ view: "browse", genre: ["Mystery"] });
  });

  it("offers no +n with three genres or fewer", async () => {
    const { main } = await renderDetail({ ...fullMovie, genres: ["Drama", "Mystery", "Thriller"] });

    expect(within(main).queryByRole("button", { name: /more genre/ })).toBeNull();
  });

  it("hides every missing line without empty separators or undefined", async () => {
    const { main } = await renderDetail(bareMovie);

    const text = main.textContent;
    expect(text).not.toContain("undefined");
    expect(text).not.toContain("Original title");
    expect(text).not.toContain("Director");
    expect(text).not.toContain("Content rating");
    expect(text).not.toMatch(/·\s*·|^\s*·|·\s*$/);
    expect(within(main).queryByRole("list", { name: "Genres" })).toBeNull();
    expect(within(main).queryByRole("button", { name: "More" })).toBeNull();
  });

  it("hides the original title when it equals the title", async () => {
    const { main } = await renderDetail({ ...fullMovie, originalTitle: fullMovie.title });

    expect(within(main).queryByText(/Original title/)).toBeNull();
  });
});

describe("DetailLayout summary", () => {
  function stubOverflow(overflows: boolean) {
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(overflows ? 200 : 80);
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(80);
  }

  it("shows the tagline above the summary clamped to four lines with More and Less", async () => {
    stubOverflow(true);
    const { main } = await renderDetail(fullMovie);

    const tagline = within(main).getByText("Some lights should never come back on.");
    const summary = within(main).getByText(/Ines takes over the lighthouse/);
    expect(
      tagline.compareDocumentPosition(summary) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(summary.className).toContain("line-clamp-4");
    const more = within(main).getByRole("button", { name: "More" });
    expect(more.getAttribute("aria-expanded")).toBe("false");
    expect(more.getAttribute("aria-controls")).toBe(summary.id);

    fireEvent.click(more);

    expect(summary.className).not.toContain("line-clamp-4");
    const less = within(main).getByRole("button", { name: "Less" });
    expect(less.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(less);
    expect(summary.className).toContain("line-clamp-4");
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("offers no toggle when the summary fits", async () => {
    stubOverflow(false);
    const { main } = await renderDetail(fullMovie);

    expect(within(main).getByText(/Ines takes over the lighthouse/)).toBeTruthy();
    expect(within(main).queryByRole("button", { name: "More" })).toBeNull();
  });
});

describe("DetailLayout per kind", () => {
  it.each([
    { kind: "extra" as const, aspect: "aspect-video" },
    { kind: "episode" as const, aspect: "aspect-video" },
    { kind: "movie" as const, aspect: "aspect-2/3" },
  ])("shows the $kind artwork as $aspect", async ({ kind, aspect }) => {
    const { main } = await renderDetail({
      ...bareMovie,
      kind,
      artwork: { thumb: { id: "still-1", tag: "s" }, poster: { id: "poster-1", tag: "p" } },
    });

    const artwork = main.querySelector("img");
    expect(artwork?.parentElement?.className).toContain(aspect);
  });
});
