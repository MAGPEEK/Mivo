import type { Row, RowsHint } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { moviesRow, showsRow } from "../test/homeFixtures";
import { homeHint, visibleHomeRows, type RowCounts } from "./homeRows";

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
