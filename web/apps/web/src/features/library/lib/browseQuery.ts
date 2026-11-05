import type { ItemKind, ItemSort, LibraryItemsQuery, LibraryKind } from "@mivo/api-client";

// URL contract of the Browse pivot (task M5-31). The search parameter names are the query
// parameter names of `listLibraryItems`, so a "See all" link or a later filter task (M5-32...36)
// can build the URL from the API query and back. Defaults are left out of the URL.

/** Quick filter of the library grid; "unmatched" is the `matchState` filter (LB-20). */
export type LibraryItemFilter = "all" | "unmatched";

/** Every quick filter, in display order. */
export const libraryItemFilters: readonly LibraryItemFilter[] = ["all", "unmatched"];

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
export interface BrowseSearch {
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
  /** Only `unmatched` ("Unmatched" quick filter). */
  readonly matchState?: "unmatched" | undefined;
}

/** The Browse state with every default filled in. */
export interface BrowseState {
  readonly type: BrowseType;
  readonly sort: ItemSort;
  readonly order: BrowseOrder;
  readonly seed: number | undefined;
  readonly filter: LibraryItemFilter;
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
  return {
    view: libraryPivots.find((pivot) => pivot === view),
    type: typeof type === "string" && browsableTypes.has(type) ? (type as BrowseType) : undefined,
    sort,
    order: !random && search["order"] === "desc" ? "desc" : undefined,
    seed: random ? parseSeed(search["seed"]) : undefined,
    matchState: search["matchState"] === "unmatched" ? "unmatched" : undefined,
  };
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
    filter: search.matchState === "unmatched" ? "unmatched" : "all",
  };
}

/**
 * The URL search of `state`; defaults are left out, so the plain library URL stays clean. The
 * pivot is not Browse state; the page keeps it.
 */
export function toBrowseSearch(state: BrowseState, kind: LibraryKind): BrowseListingSearch {
  const random = state.sort === "random";
  return {
    type: state.type === browseTypesFor(kind)[0] ? undefined : state.type,
    sort: state.sort === defaultSort ? undefined : state.sort,
    order: !random && state.order === "desc" ? "desc" : undefined,
    seed: random ? state.seed : undefined,
    matchState: state.filter === "unmatched" ? "unmatched" : undefined,
  };
}

/** The `listLibraryItems` query of `state`; only values other than the defaults are sent. */
export function toListQuery(state: BrowseState, kind: LibraryKind): BrowseListQuery {
  const { type, sort, order, seed, matchState } = toBrowseSearch(state, kind);
  // Absent keys instead of `undefined` values, so the query key stays equal to the URL.
  return {
    ...(type === undefined ? {} : { type }),
    ...(sort === undefined ? {} : { sort }),
    ...(order === undefined ? {} : { order }),
    ...(seed === undefined ? {} : { seed }),
    ...(matchState === undefined ? {} : { matchState }),
  };
}

/** The quick filter with this id; unknown ids choose "all". */
export function toItemFilter(id: string): LibraryItemFilter {
  return id === "unmatched" ? "unmatched" : "all";
}
