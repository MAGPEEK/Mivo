import type { LibraryKind } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  browseSortsFor,
  browseTypesFor,
  itemFilterOf,
  maxSeed,
  resolveBrowse,
  toBrowseSearch,
  toFacetQuery,
  toListQuery,
  validateBrowseSearch,
  type BrowseSearch,
  type BrowseState,
  type LibraryItemFilter,
} from "./browseQuery";
import { fieldFilters } from "./filterRegistry";

const emptySearch: BrowseSearch = {
  view: undefined,
  type: undefined,
  sort: undefined,
  order: undefined,
  seed: undefined,
  quick: undefined,
  matchState: undefined,
  ...Object.fromEntries(fieldFilters.map(({ id }) => [id, undefined])),
};

const allFields = fieldFilters.map(({ id }) => id);

describe("validateBrowseSearch", () => {
  it.each<{ name: string; raw: Record<string, unknown>; expected: BrowseSearch }>([
    { name: "an empty search", raw: {}, expected: emptySearch },
    {
      name: "every known value",
      raw: { type: "album", sort: "year", order: "desc", matchState: "unmatched" },
      expected: {
        ...emptySearch,
        type: "album",
        sort: "year",
        order: "desc",
        matchState: "unmatched",
      },
    },
    {
      name: "the Overview pivot",
      raw: { view: "overview" },
      expected: { ...emptySearch, view: "overview" },
    },
    {
      name: "the Browse pivot with its state",
      raw: { view: "browse", sort: "year" },
      expected: { ...emptySearch, view: "browse", sort: "year" },
    },
    { name: "an unknown pivot", raw: { view: "grid" }, expected: emptySearch },
    { name: "a pivot that is not text", raw: { view: ["overview"] }, expected: emptySearch },
    { name: "an unknown type", raw: { type: "episode" }, expected: emptySearch },
    { name: "an unknown sort", raw: { sort: "rating" }, expected: emptySearch },
    { name: "the default sort", raw: { sort: "title" }, expected: emptySearch },
    { name: "the default order", raw: { order: "asc" }, expected: emptySearch },
    { name: "an unknown order", raw: { order: "up" }, expected: emptySearch },
    { name: "an unknown match state", raw: { matchState: "matched" }, expected: emptySearch },
    { name: "a non-string value", raw: { type: 3, sort: ["year"] }, expected: emptySearch },
    {
      name: "a random seed",
      raw: { sort: "random", seed: 42 },
      expected: { ...emptySearch, sort: "random", seed: 42 },
    },
    {
      name: "a seed given as text",
      raw: { sort: "random", seed: "7" },
      expected: { ...emptySearch, sort: "random", seed: 7 },
    },
    {
      name: "the order of a random sort",
      raw: { sort: "random", order: "desc", seed: 1 },
      expected: { ...emptySearch, sort: "random", seed: 1 },
    },
    {
      name: "a seed without random sort",
      raw: { sort: "year", seed: 3 },
      expected: { ...emptySearch, sort: "year" },
    },
    {
      name: "a negative seed",
      raw: { sort: "random", seed: -1 },
      expected: { ...emptySearch, sort: "random" },
    },
    {
      name: "a fractional seed",
      raw: { sort: "random", seed: 1.5 },
      expected: { ...emptySearch, sort: "random" },
    },
    {
      name: "a seed above int32",
      raw: { sort: "random", seed: maxSeed + 1 },
      expected: { ...emptySearch, sort: "random" },
    },
    {
      name: "an empty seed",
      raw: { sort: "random", seed: " " },
      expected: { ...emptySearch, sort: "random" },
    },
    { name: "a quick filter", raw: { quick: "hdr" }, expected: { ...emptySearch, quick: "hdr" } },
    { name: "the default quick filter", raw: { quick: "all" }, expected: emptySearch },
    { name: "an unknown quick filter", raw: { quick: "unwatched" }, expected: emptySearch },
    {
      name: "a quick filter next to the match state (single choice)",
      raw: { quick: "atmos", matchState: "unmatched" },
      expected: { ...emptySearch, quick: "atmos" },
    },
    {
      name: "field values",
      raw: { genre: ["Drama", "Comedy"], decade: [1990], audioLanguage: "ger" },
      expected: {
        ...emptySearch,
        genre: ["Drama", "Comedy"],
        decade: ["1990"],
        audioLanguage: ["ger"],
      },
    },
    {
      name: "invalid field values",
      raw: { genre: ["", 3.5, {}], year: ["99", 1999, "20000"], decade: [1995, "2000"] },
      expected: { ...emptySearch, year: ["1999"], decade: ["2000"] },
    },
  ])("keeps only the valid values of $name", ({ raw, expected }) => {
    expect(validateBrowseSearch(raw)).toEqual(expected);
  });
});

describe("resolveBrowse", () => {
  it("fills in the defaults of the library kind", () => {
    expect(resolveBrowse(emptySearch, "music")).toEqual({
      type: "artist",
      sort: "title",
      order: "asc",
      seed: undefined,
      quick: "all",
      fields: {},
    });
  });

  it("falls back to the default type when the library cannot list the type", () => {
    expect(resolveBrowse({ type: "album" }, "movies").type).toBe("movie");
  });

  it("falls back to the title sort when the type does not offer the sort", () => {
    expect(resolveBrowse({ sort: "duration" }, "shows").sort).toBe("title");
  });

  it("keeps a sort the type offers", () => {
    expect(
      resolveBrowse({ type: "photo", sort: "takenAt", order: "desc" }, "photos"),
    ).toMatchObject({ type: "photo", sort: "takenAt", order: "desc" });
  });

  it("has no direction for the random sort", () => {
    expect(resolveBrowse({ sort: "random", order: "desc", seed: 4 }, "movies")).toMatchObject({
      order: "asc",
      seed: 4,
    });
  });

  it("lists the default type of an unknown library kind", () => {
    // Later API versions may add kinds; the page must still render.
    expect(resolveBrowse(emptySearch, "comics" as LibraryKind).type).toBe("movie");
  });
});

describe("toBrowseSearch and toListQuery", () => {
  const defaults: BrowseState = resolveBrowse(emptySearch, "movies");

  it.each<{ name: string; state: BrowseState; kind: LibraryKind; query: object }>([
    { name: "the defaults", state: defaults, kind: "movies", query: {} },
    {
      name: "a sort with direction",
      state: { ...defaults, sort: "dateAdded", order: "desc" },
      kind: "movies",
      query: { sort: "dateAdded", order: "desc" },
    },
    {
      name: "the random sort with its seed",
      state: { ...defaults, sort: "random", order: "desc", seed: 9 },
      kind: "movies",
      query: { sort: "random", seed: 9 },
    },
    {
      name: "a seed of another sort",
      state: { ...defaults, sort: "year", seed: 9 },
      kind: "movies",
      query: { sort: "year" },
    },
    {
      name: "the second type of a library",
      state: { ...defaults, type: "album" },
      kind: "music",
      query: { type: "album" },
    },
    {
      name: "the default type of a library",
      state: { ...defaults, type: "artist" },
      kind: "music",
      query: {},
    },
    {
      name: "the unmatched filter",
      state: { ...defaults, quick: "unmatched" },
      kind: "movies",
      query: { matchState: "unmatched" },
    },
  ])("leave the defaults out for $name", ({ state, kind, query }) => {
    expect(toListQuery(state, kind)).toStrictEqual(query);
    // The URL and the API query use the same names; the URL keeps every key for the router.
    expect(toBrowseSearch(state, kind)).toEqual({ ...emptySearch, ...query });
  });

  it("round-trips a URL through the state", () => {
    const url = { type: "photo", sort: "takenAt", order: "desc" };

    expect(toBrowseSearch(resolveBrowse(validateBrowseSearch(url), "photos"), "photos")).toEqual({
      ...emptySearch,
      ...url,
    });
  });

  it.each<{ name: string; state: Partial<BrowseState>; search: BrowseSearch; query: object }>([
    {
      name: "a server quick filter",
      state: { quick: "dolbyVision" },
      search: { quick: "dolbyVision" },
      query: { quick: "dolbyVision" },
    },
    {
      name: "text values",
      state: { fields: { genre: ["Drama", "Comedy"] } },
      search: { genre: ["Drama", "Comedy"] },
      query: { genre: ["Drama", "Comedy"] },
    },
    {
      name: "years and decades as numbers",
      state: { fields: { year: ["2008"], decade: ["1990", "2000"] } },
      search: { year: ["2008"], decade: ["1990", "2000"] },
      query: { year: [2008], decade: [1990, 2000] },
    },
    {
      name: "a field without values",
      state: { fields: { container: [] } },
      search: {},
      query: {},
    },
    {
      name: "a quick filter and fields together",
      state: { quick: "uhd", fields: { audioLanguage: ["ger"], resolution: ["4k"] } },
      search: { quick: "uhd", resolution: ["4k"], audioLanguage: ["ger"] },
      query: { quick: "uhd", resolution: ["4k"], audioLanguage: ["ger"] },
    },
  ])("maps $name to the URL and the listing", ({ state, search, query }) => {
    const full = { ...defaults, ...state };

    expect(toBrowseSearch(full, "movies")).toEqual({ ...emptySearch, ...search });
    expect(toListQuery(full, "movies")).toStrictEqual(query);
    // The URL survives the route's validation and the way back into the state.
    const url = toBrowseSearch(full, "movies");
    expect(
      toBrowseSearch(resolveBrowse(validateBrowseSearch({ ...url }), "movies"), "movies"),
    ).toEqual(url);
  });

  it("orders the field parameters like the registry, so equal filters share a cache key", () => {
    const first = toListQuery({ ...defaults, fields: { genre: ["A"], year: ["2001"] } }, "movies");
    const second = toListQuery({ ...defaults, fields: { year: ["2001"], genre: ["A"] } }, "movies");

    expect(JSON.stringify(first)).toBe(JSON.stringify(second));
  });
});

describe("toFacetQuery", () => {
  const defaults: BrowseState = resolveBrowse(emptySearch, "music");

  it("asks for every field under the type and filters, without sort and match state", () => {
    const state: BrowseState = {
      ...defaults,
      type: "album",
      sort: "year",
      order: "desc",
      quick: "duplicates",
      fields: { genre: ["Jazz"], decade: ["1960"] },
    };

    expect(toFacetQuery(state, "music")).toStrictEqual({
      field: allFields,
      type: "album",
      quick: "duplicates",
      genre: ["Jazz"],
      decade: [1960],
    });
  });

  it("leaves the Unmatched filter out, which facets cannot count", () => {
    expect(toFacetQuery({ ...defaults, quick: "unmatched" }, "music")).toStrictEqual({
      field: allFields,
    });
  });
});

describe("itemFilterOf", () => {
  it.each<{
    name: string;
    state: Pick<BrowseState, "quick" | "fields">;
    expected: LibraryItemFilter;
  }>([
    { name: "no filter", state: { quick: "all", fields: {} }, expected: "all" },
    { name: "only Unmatched", state: { quick: "unmatched", fields: {} }, expected: "unmatched" },
    { name: "a quick filter", state: { quick: "hdr", fields: {} }, expected: "filtered" },
    {
      name: "Unmatched with a field",
      state: { quick: "unmatched", fields: { genre: ["Drama"] } },
      expected: "filtered",
    },
    { name: "an emptied field", state: { quick: "all", fields: { genre: [] } }, expected: "all" },
  ])("says $expected for $name", ({ state, expected }) => {
    expect(itemFilterOf(state)).toBe(expected);
  });
});

describe("browse options per library kind", () => {
  it.each<{ kind: LibraryKind; types: readonly string[] }>([
    { kind: "movies", types: ["movie"] },
    { kind: "shows", types: ["show"] },
    { kind: "music", types: ["artist", "album"] },
    { kind: "audiobooks", types: ["audiobook"] },
    { kind: "photos", types: ["photo_album", "photo"] },
  ])("lists $types in a $kind library", ({ kind, types }) => {
    expect(browseTypesFor(kind)).toEqual(types);
  });

  it("offers no sort without a value for the type", () => {
    expect(browseSortsFor("show")).not.toContain("duration");
    expect(browseSortsFor("show")).not.toContain("resolution");
    expect(browseSortsFor("movie")).toContain("resolution");
  });

  it("offers the title and random sort for every type", () => {
    for (const type of [
      "movie",
      "show",
      "artist",
      "album",
      "audiobook",
      "photo_album",
      "photo",
    ] as const) {
      expect(browseSortsFor(type)).toEqual(expect.arrayContaining(["title", "random"]));
    }
  });
});
