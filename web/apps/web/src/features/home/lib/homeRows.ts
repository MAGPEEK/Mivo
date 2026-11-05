import type { Row, RowsHint } from "@mivo/api-client";

/** Why Home shows a short hint instead of, or below, its rows. */
export type HomeHint = "noItems" | "scanning" | "notEnoughData" | "allUnpinned" | "generic";

const serverHints: ReadonlySet<string> = new Set<HomeHint>([
  "noItems",
  "scanning",
  "notEnoughData",
]);

/**
 * The Home rows of pinned libraries, in server order (HO-07, SH-13): a row of a library the
 * user unpinned in the sidebar is left out. Rows across libraries and rows of libraries the
 * layout does not know yet (new libraries are pinned) stay.
 */
export function visibleHomeRows(
  rows: readonly Row[],
  unpinnedLibraryIds: ReadonlySet<string>,
): Row[] {
  return rows.filter(
    (row) => row.libraryId === undefined || !unpinnedLibraryIds.has(row.libraryId),
  );
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
