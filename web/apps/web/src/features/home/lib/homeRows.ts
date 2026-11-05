import type { Row, RowsHint } from "@mivo/api-client";

import type { CardFormat } from "../../../shared/lib/rowScroll";

/** Why Home shows a short hint instead of, or below, its rows. */
export type HomeHint = "noItems" | "scanning" | "notEnoughData" | "allUnpinned" | "generic";

/** A value of a "See all" search parameter, as the Browse pivot reads it from the URL. */
export type SeeAllValue = string | number | readonly (string | number)[];

/** Where "See all" of a row leads: the Browse pivot of its library with the row's query. */
export interface SeeAllTarget {
  readonly libraryId: string;
  readonly search: Readonly<Record<string, SeeAllValue>>;
}

const knownCardFormats: ReadonlySet<string> = new Set<CardFormat>(["poster", "episode", "square"]);
const serverHints: ReadonlySet<string> = new Set<HomeHint>([
  "noItems",
  "scanning",
  "notEnoughData",
]);

/** The card format of a row; formats of a newer server render as posters (API contract). */
export function rowCardFormat(format: string): CardFormat {
  return knownCardFormats.has(format) ? (format as CardFormat) : "poster";
}

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

function isPresent(value: unknown): value is SeeAllValue {
  if (Array.isArray(value)) {
    return value.length > 0;
  }
  return typeof value === "string" || typeof value === "number";
}

/**
 * The "See all" target of a row: the Browse pivot (`view=browse`) of its library with the
 * row's sort and filters under the names of the listing's query parameters. Rows across
 * libraries have no Browse pivot and get no link.
 */
export function seeAllTarget(
  row: Pick<Row, "libraryId" | "seeAllQuery">,
): SeeAllTarget | undefined {
  if (row.libraryId === undefined) {
    return undefined;
  }
  const search: Record<string, SeeAllValue> = { view: "browse" };
  for (const [name, value] of Object.entries(row.seeAllQuery ?? {})) {
    if (isPresent(value)) {
      search[name] = value;
    }
  }
  return { libraryId: row.libraryId, search };
}
