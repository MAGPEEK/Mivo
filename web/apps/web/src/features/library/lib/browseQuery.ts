import type {
  ItemKind,
  ItemSort,
  LibraryFacetsQuery,
  LibraryItemsQuery,
  LibraryKind,
} from "@mivo/api-client";

import {
  fieldFilters,
  fieldQueryValues,
  parseFieldValues,
  quickFilterById,
  quickFilters,
  type FieldFilterId,
  type FieldFilterSelection,
  type QuickFilterId,
  type ServerQuickFilter,
} from "./filterRegistry";

// URL contract of the Browse pivot (tasks M5-31, M5-32). The search parameter names are the
// query parameter names of `listLibraryItems`, so a "See all" link can build the URL from the
// API query and back. Defaults are left out of the URL.

/**
 * What the filters narrow, so an empty result can say why: nothing, only the Unmatched quick
 * filter, or any other filter.
 */
export type LibraryItemFilter = "all" | "unmatched" | "filtered";

/** The top-level item kinds a library listing accepts as `type`. */
export type BrowseType = Extract<
  ItemKind,
  "movie" | "show" | "artist" | "album" | "audiobook" | "photo_album" | "photo"
>;

/** Sort direction the toolbar offers. */
export type BrowseOrder = "asc" | "desc";

/** The two pivots of a library page (LB-01): the server's rows or the full listing. */
export type LibraryPivot = "overview" | "browse";

/** Every pivot, in tab order. */
export const libraryPivots: readonly LibraryPivot[] = ["overview", "browse"];

/** The URL search of the Browse pivot: everything but the pivot itself. */
export type BrowseListingSearch = Omit<BrowseSearch, "view">;

/** Validated search parameters of the library route; absent keys are the defaults. */
export interface BrowseSearch extends FieldFilterSelection {
  /** Pivot of the page; absent opens the pivot remembered for the library (task M5-30). */
  readonly view?: LibraryPivot | undefined;
  /** Item kind to list; absent lists the library's top-level kind. */
  readonly type?: BrowseType | undefined;
  /** Sort key; absent sorts by title. */
  readonly sort?: ItemSort | undefined;
  /** Only `desc`; ascending is the default. Absent with sort "random", which has no direction. */
  readonly order?: "desc" | undefined;
  /** Seed of sort "random", so a reload or the way back shows the same order. */
  readonly seed?: number | undefined;
  /** Quick filter of the server; absent is "all". */
  readonly quick?: ServerQuickFilter | undefined;
  /** Only `unmatched` ("Unmatched" quick filter); absent while `quick` is set (single choice). */
  readonly matchState?: "unmatched" | undefined;
}

/** The Browse state with every default filled in. */
export interface BrowseState {
  readonly type: BrowseType;
  readonly sort: ItemSort;
  readonly order: BrowseOrder;
  readonly seed: number | undefined;
  readonly quick: QuickFilterId;
  readonly fields: FieldFilterSelection;
}

/** The listing query of a Browse state, without paging. */
export type BrowseListQuery = Omit<LibraryItemsQuery, "limit" | "cursor">;

/** Sort key the server uses when none is given. */
export const defaultSort: ItemSort = "title";

/** Largest seed the API accepts (int32). */
export const maxSeed = 2_147_483_647;

const typesByLibraryKind: Readonly<Record<LibraryKind, readonly BrowseType[]>> = {
  movies: ["movie"],
  shows: ["show"],
  music: ["artist", "album"],
  audiobooks: ["audiobook"],
  photos: ["photo_album", "photo"],
};

// Rating and progress sorts follow with M6/M7; keys without a value for a type (a show has no
// duration, resolution or bit rate of its own) are not offered for it.
const sortsByType: Readonly<Record<BrowseType, readonly ItemSort[]>> = {
  movie: [
    "title",
    "year",
    "releaseDate",
    "contentRating",
    "duration",
    "dateAdded",
    "resolution",
    "bitrate",
    "fileSize",
    "random",
  ],
  show: ["title", "year", "releaseDate", "contentRating", "dateAdded", "fileSize", "random"],
  artist: ["title", "dateAdded", "random"],
  album: ["title", "year", "releaseDate", "dateAdded", "random"],
  audiobook: ["title", "year", "duration", "dateAdded", "fileSize", "random"],
  photo_album: ["title", "takenAt", "dateAdded", "random"],
  photo: ["title", "takenAt", "dateAdded", "resolution", "fileSize", "random"],
};

const browsableTypes = new Set<string>(Object.values(typesByLibraryKind).flat());
const knownSorts = new Set<string>(Object.values(sortsByType).flat());
const serverQuickFilters = new Set<string>(
  quickFilters.flatMap(({ parameter }) => (parameter?.name === "quick" ? [parameter.value] : [])),
);

/** The item kinds a library of `kind` can list, the default first; unknown kinds list none. */
export function browseTypesFor(kind: LibraryKind): readonly BrowseType[] {
  return Object.hasOwn(typesByLibraryKind, kind) ? typesByLibraryKind[kind] : [];
}

/** The sort keys offered for items of `type`, in menu order. */
export function browseSortsFor(type: BrowseType): readonly ItemSort[] {
  return sortsByType[type];
}

function parseSeed(value: unknown): number | undefined {
  // The router parses "?seed=42" to a number; a hand-edited link may still carry a string.
  const seed = typeof value === "string" && value.trim() !== "" ? Number(value) : value;
  return typeof seed === "number" && Number.isInteger(seed) && seed >= 0 && seed <= maxSeed
    ? seed
    : undefined;
}

function parseSort(value: unknown): ItemSort | undefined {
  return typeof value === "string" && value !== defaultSort && knownSorts.has(value)
    ? (value as ItemSort)
    : undefined;
}

function parseQuick(value: unknown): ServerQuickFilter | undefined {
  return typeof value === "string" && serverQuickFilters.has(value)
    ? (value as ServerQuickFilter)
    : undefined;
}

// Every registry field, `undefined` where nothing valid is given.
function parseFieldSelection(search: Readonly<Record<string, unknown>>): FieldFilterSelection {
  const entries = fieldFilters.map((definition) => [
    definition.id,
    parseFieldValues(definition, search[definition.id]),
  ]);
  return Object.fromEntries(entries) as FieldFilterSelection;
}

/**
 * Validates the parsed search of the library route. Unknown values and defaults are dropped, so
 * a hand-edited link still opens the page and the URL stays short. Every key is returned: the
 * router lays the result over the raw search, so a dropped value must be overwritten.
 */
export function validateBrowseSearch(search: Readonly<Record<string, unknown>>): BrowseSearch {
  const type = search["type"];
  const sort = parseSort(search["sort"]);
  const random = sort === "random";
  const view = search["view"];
  const quick = parseQuick(search["quick"]);
  return {
    view: libraryPivots.find((pivot) => pivot === view),
    type: typeof type === "string" && browsableTypes.has(type) ? (type as BrowseType) : undefined,
    sort,
    order: !random && search["order"] === "desc" ? "desc" : undefined,
    seed: random ? parseSeed(search["seed"]) : undefined,
    quick,
    // Quick filters are a single choice; a link with both keeps the server quick filter.
    matchState:
      quick === undefined && search["matchState"] === "unmatched" ? "unmatched" : undefined,
    ...parseFieldSelection(search),
  };
}

function quickFilterOf(search: BrowseSearch): QuickFilterId {
  const chosen = quickFilters.find(
    ({ parameter }) => parameter !== undefined && search[parameter.name] === parameter.value,
  );
  return chosen?.id ?? "all";
}

function quickFilterSearch(id: QuickFilterId): Pick<BrowseSearch, "quick" | "matchState"> {
  const parameter = quickFilterById(id)?.parameter;
  return {
    quick: parameter?.name === "quick" ? parseQuick(parameter.value) : undefined,
    matchState: parameter?.name === "matchState" ? "unmatched" : undefined,
  };
}

function fieldSelectionOf(search: BrowseSearch): FieldFilterSelection {
  return Object.fromEntries(
    fieldFilters.flatMap(({ id }) => {
      const values = search[id];
      return values === undefined ? [] : [[id, values] as const];
    }),
  );
}

/**
 * The Browse state of `search` in a library of `kind`: a type the library cannot list falls
 * back to its default type, a sort the type does not offer to the title sort.
 */
export function resolveBrowse(search: BrowseSearch, kind: LibraryKind): BrowseState {
  const types = browseTypesFor(kind);
  const defaultType = types[0] ?? "movie";
  const type = search.type !== undefined && types.includes(search.type) ? search.type : defaultType;
  const sort =
    search.sort !== undefined && browseSortsFor(type).includes(search.sort)
      ? search.sort
      : defaultSort;
  return {
    type,
    sort,
    order: sort !== "random" && search.order === "desc" ? "desc" : "asc",
    seed: sort === "random" ? search.seed : undefined,
    quick: quickFilterOf(search),
    fields: fieldSelectionOf(search),
  };
}

/** What the filters of `state` narrow; picks the texts of an empty result. */
export function itemFilterOf(state: Pick<BrowseState, "quick" | "fields">): LibraryItemFilter {
  const fieldFiltered = fieldFilters.some(({ id }) => (state.fields[id]?.length ?? 0) > 0);
  if (fieldFiltered || (state.quick !== "all" && state.quick !== "unmatched")) {
    return "filtered";
  }
  return state.quick;
}

/**
 * The URL search of `state`; defaults are left out, so the plain library URL stays clean. The
 * pivot is not Browse state; the page keeps it.
 */
export function toBrowseSearch(state: BrowseState, kind: LibraryKind): BrowseListingSearch {
  const random = state.sort === "random";
  const fields = Object.fromEntries(
    fieldFilters.map(({ id }) => {
      const values = state.fields[id];
      return [id, values === undefined || values.length === 0 ? undefined : values];
    }),
  ) as FieldFilterSelection;
  return {
    type: state.type === browseTypesFor(kind)[0] ? undefined : state.type,
    sort: state.sort === defaultSort ? undefined : state.sort,
    order: !random && state.order === "desc" ? "desc" : undefined,
    seed: random ? state.seed : undefined,
    ...quickFilterSearch(state.quick),
    ...fields,
  };
}

type FieldListQuery = Pick<BrowseListQuery, FieldFilterId>;

// The listing parameters of the chosen field values, in registry order, so equal selections give
// equal query keys.
function fieldListQuery(fields: FieldFilterSelection): FieldListQuery {
  const entries = fieldFilters.flatMap((definition) => {
    const values = fields[definition.id];
    return values === undefined || values.length === 0
      ? []
      : [[definition.id, fieldQueryValues(definition, values)]];
  });
  // Each entry holds the parameter type of its field (years and decades as numbers).
  return Object.fromEntries(entries) as FieldListQuery;
}

/** The `listLibraryItems` query of `state`; only values other than the defaults are sent. */
export function toListQuery(state: BrowseState, kind: LibraryKind): BrowseListQuery {
  const { type, sort, order, seed, quick, matchState } = toBrowseSearch(state, kind);
  // Absent keys instead of `undefined` values, so the query key stays equal to the URL.
  return {
    ...(type === undefined ? {} : { type }),
    ...(sort === undefined ? {} : { sort }),
    ...(order === undefined ? {} : { order }),
    ...(seed === undefined ? {} : { seed }),
    ...(quick === undefined ? {} : { quick }),
    ...(matchState === undefined ? {} : { matchState }),
    ...fieldListQuery(state.fields),
  };
}

/**
 * The `listLibraryFacets` query of `state`: every registry field under the current type and
 * filters. The facets endpoint has no match state filter, so "Unmatched" does not narrow the
 * counts.
 */
export function toFacetQuery(state: BrowseState, kind: LibraryKind): LibraryFacetsQuery {
  const { type, quick } = toBrowseSearch(state, kind);
  return {
    field: fieldFilters.map(({ id }) => id),
    ...(type === undefined ? {} : { type }),
    ...(quick === undefined ? {} : { quick }),
    ...fieldListQuery(state.fields),
  };
}
