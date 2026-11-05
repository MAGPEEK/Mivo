import type { MatchState } from "@mivo/api-client";
import { useCallback, useState } from "react";

/** Quick filters of the library grid (LB-20); more follow with the filter menu. */
export type LibraryItemFilter = "all" | "unmatched";

/** Every quick filter, in display order. */
export const libraryItemFilters: readonly LibraryItemFilter[] = ["all", "unmatched"];

/** The chosen quick filter of the library grid. */
export interface ItemFilter {
  readonly filter: LibraryItemFilter;
  /** The `matchState` query of the filter; undefined lists every item. */
  readonly matchState: MatchState | undefined;
  /** Chooses the filter with this id; unknown ids choose "all". */
  readonly select: (id: string) => void;
}

function toFilter(id: string): LibraryItemFilter {
  return id === "unmatched" ? "unmatched" : "all";
}

/** State of the library grid's quick filter; it starts at "all". */
export function useItemFilter(): ItemFilter {
  const [filter, setFilter] = useState<LibraryItemFilter>("all");
  const select = useCallback((id: string) => {
    setFilter(toFilter(id));
  }, []);
  return { filter, matchState: filter === "unmatched" ? "unmatched" : undefined, select };
}
