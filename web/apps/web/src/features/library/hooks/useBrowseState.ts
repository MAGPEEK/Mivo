import type { ItemSort, LibraryKind } from "@mivo/api-client";
import { useCallback, useEffect, useMemo } from "react";

import {
  browseSortsFor,
  browseTypesFor,
  maxSeed,
  resolveBrowse,
  toBrowseSearch,
  toItemFilter,
  toListQuery,
  type BrowseListQuery,
  type BrowseSearch,
  type BrowseState,
  type BrowseType,
} from "../lib/browseQuery";

/** How a search change enters the history. */
export interface BrowseNavigation {
  /** Replaces the current entry instead of adding one (used for the generated random seed). */
  readonly replace?: boolean;
}

/** Applies a new URL search to the library route. */
export type BrowseSearchChange = (next: BrowseSearch, navigation?: BrowseNavigation) => void;

/** Options of {@link useBrowseState}. */
export interface BrowseStateOptions {
  readonly search: BrowseSearch;
  readonly onSearchChange: BrowseSearchChange;
  readonly libraryKind: LibraryKind;
  /** Source of new random seeds; tests pass a fixed one. */
  readonly nextSeed?: () => number;
}

/** The Browse toolbar state from the URL and the actions that change it. */
export interface BrowseControls extends BrowseState {
  /** The item kinds the library can list; the type switch shows only with more than one. */
  readonly types: readonly BrowseType[];
  readonly sorts: readonly ItemSort[];
  /** The listing query of the state; a new value starts again at the first page. */
  readonly query: BrowseListQuery;
  readonly selectType: (type: string) => void;
  /** Choosing "random" again shuffles again with a new seed. */
  readonly selectSort: (sort: string) => void;
  readonly toggleOrder: () => void;
  readonly selectFilter: (filter: string) => void;
}

function randomSeed(): number {
  return Math.floor(Math.random() * (maxSeed + 1));
}

function isOneOf<Value extends string>(values: readonly Value[], id: string): id is Value {
  return (values as readonly string[]).includes(id);
}

/**
 * State of the Browse toolbar (LB-20...23). It lives entirely in the URL: every change goes
 * through `onSearchChange`, which also resets the scroll position, and the new listing query
 * starts again without a cursor.
 */
export function useBrowseState({
  search,
  onSearchChange,
  libraryKind,
  nextSeed = randomSeed,
}: BrowseStateOptions): BrowseControls {
  const state = useMemo(() => resolveBrowse(search, libraryKind), [search, libraryKind]);
  const query = useMemo(() => toListQuery(state, libraryKind), [state, libraryKind]);
  const apply = useCallback(
    (next: BrowseState, navigation?: BrowseNavigation) => {
      onSearchChange(toBrowseSearch(next, libraryKind), navigation);
    },
    [onSearchChange, libraryKind],
  );
  // A random sort without a seed (a hand-edited link) gets one, so a reload keeps its order.
  const missingSeed = state.sort === "random" && state.seed === undefined;
  useEffect(() => {
    if (missingSeed) {
      apply({ ...state, seed: nextSeed() }, { replace: true });
    }
  }, [missingSeed, state, apply, nextSeed]);
  const types = browseTypesFor(libraryKind);
  const sorts = browseSortsFor(state.type);
  return {
    ...state,
    types,
    sorts,
    query,
    selectType: (id) => {
      if (isOneOf(types, id) && id !== state.type) {
        const sort = browseSortsFor(id).includes(state.sort) ? state.sort : "title";
        apply({ ...state, type: id, sort });
      }
    },
    selectSort: (id) => {
      if (isOneOf(sorts, id)) {
        apply({ ...state, sort: id, seed: id === "random" ? nextSeed() : undefined });
      }
    },
    toggleOrder: () => {
      apply({ ...state, order: state.order === "asc" ? "desc" : "asc" });
    },
    selectFilter: (id) => {
      apply({ ...state, filter: toItemFilter(id) });
    },
  };
}
