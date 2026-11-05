import type { ItemSummary, Row, RowsHint } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeItem, fakePoster, fakeShow } from "../../../test/libraryFixtures";
import { moviesRow, showsRow } from "../test/homeFixtures";
import { homeHint, rowCardFormat, seeAllTarget, visibleHomeRows, type RowCounts } from "./homeRows";
import { rowCardArtwork, toRowCardModel } from "./rowCard";

const thumb = { id: "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", tag: "thumb" };
const square = { id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e", tag: "square" };

describe("rowCardFormat", () => {
  it.each([
    ["poster", "poster"],
    ["episode", "episode"],
    ["square", "square"],
    ["banner", "poster"],
  ])("maps %s to %s", (format, expected) => {
    expect(rowCardFormat(format)).toBe(expected);
  });
});

describe("visibleHomeRows", () => {
  const acrossLibraries: Row = {
    id: "continueWatching",
    kind: "recentlyAdded",
    titleKey: "rows.recentlyAdded",
    cardFormat: "poster",
    items: [],
  };

  it("keeps server order and drops rows of unpinned libraries", () => {
    const rows = [moviesRow, acrossLibraries, showsRow];

    expect(visibleHomeRows(rows, new Set([moviesRow.libraryId ?? ""]))).toEqual([
      acrossLibraries,
      showsRow,
    ]);
    expect(visibleHomeRows(rows, new Set())).toEqual(rows);
  });
});

describe("homeHint", () => {
  it.each<[string, RowsHint | undefined, RowCounts, ReturnType<typeof homeHint>]>([
    ["rows without a hint", undefined, { server: 2, visible: 2 }, undefined],
    ["every row unpinned", undefined, { server: 2, visible: 0 }, "allUnpinned"],
    ["the server's reason", "noItems", { server: 0, visible: 0 }, "noItems"],
    ["a running first scan", "scanning", { server: 0, visible: 0 }, "scanning"],
    ["missing metadata next to rows", "notEnoughData", { server: 1, visible: 1 }, "notEnoughData"],
    ["a hint of a newer server", "tooSoon" as RowsHint, { server: 0, visible: 0 }, "generic"],
    ["no rows and no reason", undefined, { server: 0, visible: 0 }, "generic"],
  ])("%s", (_, hint, counts, expected) => {
    expect(homeHint(hint, counts)).toBe(expected);
  });
});

describe("seeAllTarget", () => {
  it("opens the Browse pivot with the row's query", () => {
    expect(seeAllTarget(showsRow)).toEqual({
      libraryId: showsRow.libraryId,
      search: { view: "browse", sort: "dateAdded", order: "desc", type: "show" },
    });
  });

  it("keeps filled lists and drops empty ones", () => {
    const target = seeAllTarget({
      libraryId: "lib",
      seeAllQuery: { sort: "random", seed: 42, genre: ["Comedy"], year: [] },
    });

    expect(target?.search).toEqual({ view: "browse", sort: "random", seed: 42, genre: ["Comedy"] });
  });

  it("opens Browse with its defaults without a query", () => {
    expect(seeAllTarget({ libraryId: "lib" })?.search).toEqual({ view: "browse" });
  });

  it("has no target for rows across libraries", () => {
    expect(seeAllTarget({})).toBeUndefined();
  });
});

function season(overrides: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id: "season-1",
    libraryId: "lib",
    kind: "season",
    title: "Season 1",
    number: 1,
    showTitle: "Elephants Dream",
    ...overrides,
  };
}

function episode(overrides: Partial<ItemSummary> = {}): ItemSummary {
  return fakeItem({
    id: "episode-1",
    kind: "episode",
    title: "Pilot",
    number: 9,
    seasonNumber: 3,
    showTitle: "Elephants Dream",
    ...overrides,
  });
}

describe("toRowCardModel", () => {
  it("names a grouped show card by its new episodes", () => {
    const card = toRowCardModel(fakeShow({ newEpisodeCount: 3 }));

    expect(card).toMatchObject({
      title: "Big Buck Bunny Show",
      detail: { kind: "newEpisodes", count: 3 },
      newEpisodeCount: 3,
      isSeries: true,
      available: true,
    });
  });

  it("describes a show without new episodes by year and seasons", () => {
    expect(toRowCardModel(fakeShow()).detail).toEqual({
      kind: "show",
      year: "2015",
      seasonCount: 3,
    });
    const bareShow: ItemSummary = { id: "show", libraryId: "lib", kind: "show", title: "Show" };
    expect(toRowCardModel(bareShow).detail).toEqual({
      kind: "show",
      year: "",
      seasonCount: 0,
    });
  });

  it("names a season card after its show", () => {
    const card = toRowCardModel(season({ newEpisodeCount: 2 }));

    expect(card).toMatchObject({
      title: "Elephants Dream",
      detail: { kind: "season", seasonNumber: 1, name: "Season 1" },
      newEpisodeCount: 2,
    });
  });

  it("keeps the season title without a show title", () => {
    expect(toRowCardModel(season({ showTitle: "" })).title).toBe("Season 1");
  });

  it("labels an episode with its numbers and own title", () => {
    const card = toRowCardModel(episode({ numberEnd: 10 }));

    expect(card).toMatchObject({
      title: "Elephants Dream",
      detail: {
        kind: "episode",
        numbers: { season: 3, number: 9, numberEnd: 10 },
        episodeTitle: "Pilot",
      },
      newEpisodeCount: undefined,
      available: true,
    });
    expect(toRowCardModel(episode({ showTitle: "" })).detail).toMatchObject({
      episodeTitle: "",
    });
  });

  it("marks a movie without a playable file unavailable", () => {
    const movie: ItemSummary = { id: "m", libraryId: "lib", kind: "movie", title: "M", year: 2008 };
    const card = toRowCardModel(movie);

    expect(card).toMatchObject({
      detail: { kind: "other", year: "2008" },
      isSeries: false,
      available: false,
    });
  });
});

describe("rowCardArtwork", () => {
  const item = fakeItem({ poster: fakePoster, thumb, square });

  it.each([
    ["poster", fakePoster, "poster"],
    ["episode", thumb, "still"],
    ["square", square, "square"],
  ] as const)("picks the %s image", (format, image, aspect) => {
    expect(rowCardArtwork(item, format)).toEqual({ imageRef: image, aspect });
  });

  it("falls back to the poster", () => {
    expect(rowCardArtwork(fakeItem({ poster: fakePoster }), "episode").imageRef).toBe(fakePoster);
    expect(rowCardArtwork(fakeItem({ poster: fakePoster }), "square").imageRef).toBe(fakePoster);
  });
});
