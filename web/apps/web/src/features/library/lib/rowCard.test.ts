import type { ItemSummary } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeItem, fakePoster, fakeShow } from "../../../test/libraryFixtures";
import { rowCardArtwork, toRowCardModel } from "./rowCard";

const thumb = { id: "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", tag: "thumb" };
const square = { id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e", tag: "square" };

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

  it("keeps season 0 (specials) and falls back to the season number of an unnumbered season", () => {
    expect(toRowCardModel(season({ number: 0, title: "Specials" })).detail).toEqual({
      kind: "season",
      seasonNumber: 0,
      name: "Specials",
    });
    const unnumbered = season({ title: "Extras" });
    delete unnumbered.number;
    expect(toRowCardModel({ ...unnumbered, seasonNumber: 4 }).detail).toEqual({
      kind: "season",
      seasonNumber: 4,
      name: "Extras",
    });
    expect(toRowCardModel(unnumbered).detail).toEqual({
      kind: "season",
      seasonNumber: undefined,
      name: "Extras",
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
