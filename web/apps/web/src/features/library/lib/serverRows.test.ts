import type { Row, RowsHint } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { validateBrowseSearch } from "./browseQuery";
import { overviewHint, rowCardFormat, seeAllTarget } from "./serverRows";

const libraryId = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c";

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

describe("seeAllTarget", () => {
  it("opens the Browse pivot with the row's query", () => {
    expect(
      seeAllTarget({ libraryId, seeAllQuery: { sort: "dateAdded", order: "desc", type: "show" } }),
    ).toEqual({
      libraryId,
      search: { view: "browse", sort: "dateAdded", order: "desc", type: "show" },
    });
  });

  it("keeps a random seed, the genre and the quick filter and drops empty filters", () => {
    const target = seeAllTarget({
      libraryId,
      seeAllQuery: { sort: "random", seed: 42, genre: ["Comedy"], year: [], quick: "hdr" },
    });

    expect(target?.search).toEqual({
      view: "browse",
      sort: "random",
      seed: 42,
      genre: ["Comedy"],
      quick: "hdr",
    });
  });

  it("carries years and decades as URL text", () => {
    const target = seeAllTarget({ libraryId, seeAllQuery: { year: [2008], decade: [1990] } });

    expect(target?.search).toEqual({ view: "browse", year: ["2008"], decade: ["1990"] });
  });

  it("opens Browse even when the query names a pivot", () => {
    const query = { view: "overview" } as unknown as NonNullable<Row["seeAllQuery"]>;

    expect(seeAllTarget({ libraryId, seeAllQuery: query })?.search).toEqual({ view: "browse" });
  });

  it("opens Browse with its defaults without a query", () => {
    expect(seeAllTarget({ libraryId })?.search).toEqual({ view: "browse" });
  });

  it("has no target for rows across libraries", () => {
    expect(seeAllTarget({})).toBeUndefined();
  });

  it("passes the route's validation unchanged", () => {
    const search = seeAllTarget({
      libraryId,
      seeAllQuery: { sort: "year", order: "desc", matchState: "unmatched", type: "movie" },
    })?.search;

    expect(validateBrowseSearch({ ...search })).toEqual(search);
  });
});

describe("overviewHint", () => {
  it.each<[string, RowsHint | undefined, number, ReturnType<typeof overviewHint>]>([
    ["rows without a hint", undefined, 2, undefined],
    ["the server's reason", "noItems", 0, "noItems"],
    ["a running first scan", "scanning", 0, "scanning"],
    ["missing metadata next to rows", "notEnoughData", 1, "notEnoughData"],
    ["a Home-only hint", "noLibraries", 0, "generic"],
    ["a hint of a newer server", "tooSoon" as RowsHint, 0, "generic"],
    ["no rows and no reason", undefined, 0, "generic"],
  ])("%s", (_, hint, rowCount, expected) => {
    expect(overviewHint(hint, rowCount)).toBe(expected);
  });
});
