import type { HomeRows, Library, Row } from "@mivo/api-client";

import { jsonResponse, problemResponse, type FakeRoute } from "../../../test/fakeApi";
import { fakeItem, fakeLibrary, fakeShow } from "../../../test/libraryFixtures";

/** The movies library of the Home tests. */
export const moviesLibrary: Library = fakeLibrary({ name: "Movies" });

/** The shows library of the Home tests. */
export const showsLibrary: Library = fakeLibrary({
  id: "9d8c7b6a-5f4e-4d3c-8b2a-1f0e9d8c7b6a",
  name: "Shows",
  kind: "shows",
});

/** "Recently added in Movies" with two movies. */
export const moviesRow: Row = {
  id: `recentlyAdded:${moviesLibrary.id}`,
  kind: "recentlyAdded",
  titleKey: "rows.recentlyAddedIn",
  titleParams: { library: "Movies" },
  libraryId: moviesLibrary.id,
  cardFormat: "poster",
  seeAllQuery: { sort: "dateAdded", order: "desc" },
  items: [
    fakeItem({ id: "movie-1", title: "Sintel", year: 2010 }),
    fakeItem({ id: "movie-2", title: "Tears of Steel", year: 2012 }),
  ],
};

/** "Recently added in Shows" with a grouped show card and a grouped season card. */
export const showsRow: Row = {
  id: `recentlyAdded:${showsLibrary.id}`,
  kind: "recentlyAdded",
  titleKey: "rows.recentlyAddedIn",
  titleParams: { library: "Shows" },
  libraryId: showsLibrary.id,
  cardFormat: "poster",
  seeAllQuery: { sort: "dateAdded", order: "desc", type: "show" },
  items: [
    fakeShow({
      id: "show-1",
      libraryId: showsLibrary.id,
      title: "Cosmos Laundromat",
      newEpisodeCount: 3,
    }),
    {
      id: "season-2",
      libraryId: showsLibrary.id,
      kind: "season",
      title: "Season 2",
      number: 2,
      showTitle: "Elephants Dream",
      newEpisodeCount: 4,
    },
  ],
};

/** Home rows of both libraries. */
export const twoLibraryRows: HomeRows = { rows: [moviesRow, showsRow], hero: [] };

/** Answers `GET /api/v1/home/rows` with `rows`. */
export function homeRowsRoute(rows: HomeRows): FakeRoute {
  return { method: "GET", path: "/api/v1/home/rows", respond: () => jsonResponse(rows) };
}

/** Answers `GET /api/v1/home/rows` with a server error until `healed()` returns true. */
export function failingHomeRowsRoute(rows: HomeRows, healed: () => boolean): FakeRoute {
  return {
    method: "GET",
    path: "/api/v1/home/rows",
    respond: () => (healed() ? jsonResponse(rows) : problemResponse(500, "internal")),
  };
}
