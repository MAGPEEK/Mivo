import type { ItemSort, LibraryFacetsQuery, LibraryKind } from "@mivo/api-client";
import { useCallback, useEffect, useMemo } from "react";

import {
  browseSortsFor,
  browseTypesFor,
  defaultSort,
  itemFilterOf,
  maxSeed,
  resolveBrowse,
  toBrowseSearch,
  toFacetQuery,
  toListQuery,
  type BrowseListQuery,
  type BrowseSearch,
  type BrowseState,
  type BrowseType,
  type LibraryItemFilter,
} from "../lib/browseQuery";
import {
  quickFilterById,
  toggleFieldValue,
  type FieldFilterId,
  type FieldFilterSelection,
} from "../lib/filterRegistry";

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

/** The filter actions of the Browse toolbar (LB-20). */
export interface BrowseFilterActions {
  /** Chooses a quick filter by id; unknown ids and the current one are ignored. */
  readonly selectQuick: (id: string) => void;
  /** Adds the value to the field's filter, or removes it when it is chosen already. */
  readonly toggleFieldValue: (field: FieldFilterId, value: string) => void;
  /** Removes every quick and field filter. */
  readonly clearFilters: () => void;
}

/** The Browse toolbar state from the URL and the actions that change it. */
export interface BrowseControls extends BrowseState, BrowseFilterActions {
  /** The item kinds the library can list; the type switch shows only with more than one. */
  readonly types: readonly BrowseType[];
  readonly sorts: readonly ItemSort[];
  /** What the filters narrow, for the texts of an empty result. */
  readonly filter: LibraryItemFilter;
  /**
   * The listing query of the state; a new value starts again at the first page. `undefined`
   * while a random sort waits for its seed, so no unseeded listing is requested.
   */
  readonly query: BrowseListQuery | undefined;
  /** The facet query of the current type and filters, for the counts of the filter menu. */
  readonly facetQuery: LibraryFacetsQuery;
  readonly selectType: (type: string) => void;
  /** Choosing "random" again shuffles again with a new seed. */
  readonly selectSort: (sort: string) => void;
  readonly toggleOrder: () => void;
}

const noFields: FieldFilterSelection = {};

function randomSeed(): number {
  return Math.floor(Math.random() * (maxSeed + 1));
}

function isOneOf<Value extends string>(values: readonly Value[], id: string): id is Value {
  return (values as readonly string[]).includes(id);
}

function filterActions(
  state: BrowseState,
  apply: (next: BrowseState) => void,
): BrowseFilterActions {
  return {
    selectQuick: (id) => {
      const quick = quickFilterById(id)?.id;
      if (quick !== undefined && quick !== state.quick) {
        apply({ ...state, quick });
      }
    },
    toggleFieldValue: (field, value) => {
      apply({ ...state, fields: toggleFieldValue(state.fields, field, value) });
    },
    clearFilters: () => {
      apply({ ...state, quick: "all", fields: noFields });
    },
  };
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
  // A random sort without a seed (a hand-edited link) gets one, so a reload keeps its order.
  const missingSeed = state.sort === "random" && state.seed === undefined;
  const query = useMemo(
    () => (missingSeed ? undefined : toListQuery(state, libraryKind)),
    [missingSeed, state, libraryKind],
  );
  const facetQuery = useMemo(() => toFacetQuery(state, libraryKind), [state, libraryKind]);
  const apply = useCallback(
    (next: BrowseState, navigation?: BrowseNavigation) => {
      onSearchChange(toBrowseSearch(next, libraryKind), navigation);
    },
    [onSearchChange, libraryKind],
  );
  useEffect(() => {
    if (missingSeed) {
      apply({ ...state, seed: nextSeed() }, { replace: true });
    }
  }, [missingSeed, state, apply, nextSeed]);
  const types = browseTypesFor(libraryKind);
  const sorts = browseSortsFor(state.type);
  return {
    ...state,
    ...filterActions(state, apply),
    types,
    sorts,
    filter: itemFilterOf(state),
    query,
    facetQuery,
    selectType: (id) => {
      if (isOneOf(types, id) && id !== state.type) {
        const sort = browseSortsFor(id).includes(state.sort) ? state.sort : defaultSort;
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
  };
}
