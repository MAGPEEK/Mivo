import { describe, expect, it } from "vitest";

import { rowTitleMessage, type RowTitleSource } from "./rowTitle";

describe("rowTitleMessage", () => {
  it.each<[string, RowTitleSource, ReturnType<typeof rowTitleMessage>]>([
    [
      "a key with its parameter",
      {
        kind: "recentlyAdded",
        titleKey: "rows.recentlyAddedIn",
        titleParams: { library: "Movies" },
      },
      { key: "rows.recentlyAddedIn", values: { library: "Movies" } },
    ],
    [
      "a key without its parameter falls back to the kind",
      { kind: "recentlyAdded", titleKey: "rows.recentlyAddedIn" },
      { key: "rows.recentlyAdded" },
    ],
    [
      "a blank parameter counts as missing",
      { kind: "recentlyAdded", titleKey: "rows.recentlyAddedIn", titleParams: { library: " " } },
      { key: "rows.recentlyAdded" },
    ],
    [
      "a plain key",
      { kind: "newReleases", titleKey: "rows.newReleases" },
      { key: "rows.newReleases" },
    ],
    [
      "the genre from the parameters",
      { kind: "moreGenre", titleKey: "rows.moreGenre", titleParams: { genre: "Comedy" } },
      { key: "rows.moreGenre", values: { genre: "Comedy" } },
    ],
    [
      "the genre from the row",
      { kind: "moreGenre", titleKey: "rows.moreGenre", genre: "Drama" },
      { key: "rows.moreGenre", values: { genre: "Drama" } },
    ],
    [
      "an unknown key of a known kind",
      { kind: "recentEpisodes", titleKey: "rows.recentEpisodesIn" },
      { key: "rows.recentEpisodes" },
    ],
    [
      "an unknown key of a genre row",
      { kind: "moreGenre", titleKey: "rows.genreMix", genre: "Drama" },
      { key: "rows.moreGenre", values: { genre: "Drama" } },
    ],
    [
      "a genre row without a genre",
      { kind: "moreGenre", titleKey: "rows.genreMix" },
      { key: "rows.fallback" },
    ],
    [
      "an unknown kind and key",
      { kind: "topRated" as RowTitleSource["kind"], titleKey: "rows.topRated" },
      { key: "rows.fallback" },
    ],
    [
      "a key and kind named like Object.prototype members",
      { kind: "constructor" as RowTitleSource["kind"], titleKey: "toString" },
      { key: "rows.fallback" },
    ],
  ])("%s", (_, row, expected) => {
    expect(rowTitleMessage(row)).toEqual(expected);
  });
});
