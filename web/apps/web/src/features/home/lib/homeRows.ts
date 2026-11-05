import type { HeroItem, Row, RowsHint } from "@mivo/api-client";

/** Why Home shows a short hint instead of, or below, its rows. */
export type HomeHint = "noItems" | "scanning" | "notEnoughData" | "allUnpinned" | "generic";

const serverHints: ReadonlySet<string> = new Set<HomeHint>([
  "noItems",
  "scanning",
  "notEnoughData",
]);

/** The sidebar layout as Home applies it to its rows and hero. */
export interface HomeLayout {
  /** Library IDs in sidebar order; empty while the libraries load or failed. */
  readonly order: readonly string[];
  /** Libraries the user unpinned in the sidebar. */
  readonly unpinned: ReadonlySet<string>;
}

// Rows across libraries (continue watching, M7) lead; rows of a library the layout does not
// know yet follow the arranged ones.
const acrossLibrariesPosition = -1;

/**
 * The Home rows of pinned libraries in sidebar order (HO-07, SH-13): a row of a library the
 * user unpinned in the sidebar is left out, the others follow the order the user arranged.
 * Rows across libraries come first; rows of libraries the layout does not know yet (new
 * libraries are pinned) come last. Equal positions keep server order.
 */
export function visibleHomeRows(rows: readonly Row[], layout: HomeLayout): Row[] {
  const positions = new Map(layout.order.map((libraryId, index) => [libraryId, index]));
  const unknownPosition = layout.order.length;
  const position = (row: Row) =>
    row.libraryId === undefined
      ? acrossLibrariesPosition
      : (positions.get(row.libraryId) ?? unknownPosition);
  return rows
    .filter((row) => row.libraryId === undefined || !layout.unpinned.has(row.libraryId))
    .map((row, index) => ({ row, index, position: position(row) }))
    .sort((left, right) => left.position - right.position || left.index - right.index)
    .map((entry) => entry.row);
}

/** The hero titles of pinned libraries, in server order (HO-01). */
export function visibleHeroItems(
  items: readonly HeroItem[],
  unpinned: ReadonlySet<string>,
): HeroItem[] {
  return items.filter((item) => !unpinned.has(item.libraryId));
}

/** Counts of the rows the server sent and of those Home shows. */
export interface RowCounts {
  readonly server: number;
  readonly visible: number;
}

/**
 * The hint Home shows: unpinned libraries hid every row, or the server named a reason, or
 * nothing is left to show; `undefined` while rows are shown and nothing is missing. Hints of a
 * newer server become the generic hint.
 */
export function homeHint(
  serverHint: RowsHint | undefined,
  counts: RowCounts,
): HomeHint | undefined {
  if (counts.visible === 0 && counts.server > 0) {
    return "allUnpinned";
  }
  if (serverHint !== undefined) {
    return serverHints.has(serverHint) ? (serverHint as HomeHint) : "generic";
  }
  return counts.visible === 0 ? "generic" : undefined;
}
