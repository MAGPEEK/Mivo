import type { Row, RowsHint } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { heroMovie, heroShow, moviesRow, showsRow } from "../test/homeFixtures";
import { homeHint, visibleHeroItems, visibleHomeRows, type RowCounts } from "./homeRows";

describe("visibleHomeRows", () => {
  const acrossLibraries: Row = {
    id: "continueWatching",
    kind: "recentlyAdded",
    titleKey: "rows.recentlyAdded",
    cardFormat: "poster",
    items: [],
  };

  const moviesId = moviesRow.libraryId ?? "";
  const showsId = showsRow.libraryId ?? "";

  it("drops rows of unpinned libraries", () => {
    const rows = [moviesRow, acrossLibraries, showsRow];

    expect(
      visibleHomeRows(rows, { order: [moviesId, showsId], unpinned: new Set([moviesId]) }),
    ).toEqual([acrossLibraries, showsRow]);
  });

  it("keeps server order without a known layout", () => {
    const rows = [showsRow, acrossLibraries, moviesRow];

    expect(visibleHomeRows(rows, { order: [], unpinned: new Set() })).toEqual([
      acrossLibraries,
      showsRow,
      moviesRow,
    ]);
  });

  it("follows the sidebar order, rows across libraries first, unknown libraries last", () => {
    const newLibraryRow: Row = { ...moviesRow, id: "recentlyAdded:new", libraryId: "new" };
    const rows = [newLibraryRow, moviesRow, acrossLibraries, showsRow];

    expect(visibleHomeRows(rows, { order: [showsId, moviesId], unpinned: new Set() })).toEqual([
      acrossLibraries,
      showsRow,
      moviesRow,
      newLibraryRow,
    ]);
  });
});

describe("visibleHeroItems", () => {
  it("drops titles of unpinned libraries and keeps server order", () => {
    const items = [heroShow, heroMovie];

    expect(visibleHeroItems(items, new Set())).toEqual(items);
    expect(visibleHeroItems(items, new Set([heroShow.libraryId]))).toEqual([heroMovie]);
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
