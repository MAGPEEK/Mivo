import { useCallback, useEffect } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import { libraryPivots, type BrowseSearch, type LibraryPivot } from "../lib/browseQuery";
import { readRememberedPivot, rememberPivot, resolvePivot } from "../lib/pivotMemory";
import type { BrowseSearchChange } from "./useBrowseState";

/** Options of {@link useLibraryPivot}. */
export interface LibraryPivotOptions {
  readonly libraryId: string;
  readonly search: BrowseSearch;
  readonly onSearchChange: BrowseSearchChange;
  /** Where the pivot is remembered; defaults to this device's `localStorage`. */
  readonly storage?: Storage | undefined;
}

/** The pivot of a library page and the actions that change it. */
export interface LibraryPivotControls {
  readonly pivot: LibraryPivot;
  /** Opens a pivot by id; unknown ids and the current pivot are ignored. */
  readonly selectPivot: (id: string) => void;
  /** Search changes of the Browse pivot; they keep the pivot in the URL as it is. */
  readonly onBrowseSearchChange: BrowseSearchChange;
}

/**
 * The pivot of a library page (LB-01): `?view=` in the URL, else the pivot last used for this
 * library on this device, else Browse. Whatever is shown is remembered, so a "See all" link
 * or a bookmark with `view` also counts as use.
 */
export function useLibraryPivot({
  libraryId,
  search,
  onSearchChange,
  storage = browserLocalStorage(),
}: LibraryPivotOptions): LibraryPivotControls {
  const pivot = resolvePivot(search.view, readRememberedPivot(storage, libraryId));
  useEffect(() => {
    rememberPivot(storage, libraryId, pivot);
  }, [storage, libraryId, pivot]);
  const view = search.view;
  const onBrowseSearchChange = useCallback<BrowseSearchChange>(
    (next, navigation) => {
      onSearchChange({ ...next, view }, navigation);
    },
    [onSearchChange, view],
  );
  const selectPivot = useCallback(
    (id: string) => {
      const next = libraryPivots.find((candidate) => candidate === id);
      if (next !== undefined && next !== pivot) {
        // The Browse state stays in the URL, so switching back shows the same listing.
        onSearchChange({ ...search, view: next });
      }
    },
    [onSearchChange, search, pivot],
  );
  return { pivot, selectPivot, onBrowseSearchChange };
}
