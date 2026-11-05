import type { Row, RowsHint } from "@mivo/api-client";

import type { CardFormat } from "../../../shared/lib/rowScroll";
import { validateBrowseSearch, type BrowseSearch } from "./browseQuery";

// Rows the server generates for Home and for a library's Overview pivot (HO-07, LB-02...04,
// LB-09). The client renders them as they come and never builds row content itself.

/** Where "See all" of a row leads: the Browse pivot of its library with the row's query. */
export interface SeeAllTarget {
  readonly libraryId: string;
  readonly search: BrowseSearch;
}

/** Why the Overview pivot shows a short hint instead of, or below, its rows. */
export type OverviewHint = "noItems" | "scanning" | "notEnoughData" | "generic";

const knownCardFormats: ReadonlySet<string> = new Set<CardFormat>(["poster", "episode", "square"]);
const overviewServerHints: ReadonlySet<string> = new Set<OverviewHint>([
  "noItems",
  "scanning",
  "notEnoughData",
]);

/** The card format of a row; formats of a newer server render as posters (API contract). */
export function rowCardFormat(format: string): CardFormat {
  return knownCardFormats.has(format) ? (format as CardFormat) : "poster";
}

/**
 * The "See all" target of a row: the Browse pivot (`view=browse`) of its library with the
 * row's sort and filters. The query goes through the route's own validation, so the link
 * carries exactly what Browse understands (parameters of later filter tasks join once Browse
 * reads them). Rows across libraries have no Browse pivot and get no link.
 */
export function seeAllTarget(
  row: Pick<Row, "libraryId" | "seeAllQuery">,
): SeeAllTarget | undefined {
  if (row.libraryId === undefined) {
    return undefined;
  }
  // The pivot comes last, so no query key can override it.
  const search: BrowseSearch = { ...validateBrowseSearch(row.seeAllQuery ?? {}), view: "browse" };
  return { libraryId: row.libraryId, search };
}

/**
 * The hint of the Overview pivot: the reason the server named, or a generic hint when there
 * are no rows at all; `undefined` while rows are shown and nothing is missing. Hints of a newer
 * server (or "noLibraries", which only Home sends) become the generic hint.
 */
export function overviewHint(
  serverHint: RowsHint | undefined,
  rowCount: number,
): OverviewHint | undefined {
  if (serverHint !== undefined) {
    return overviewServerHints.has(serverHint) ? (serverHint as OverviewHint) : "generic";
  }
  return rowCount === 0 ? "generic" : undefined;
}
