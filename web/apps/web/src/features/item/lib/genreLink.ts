/** How many genres the title block shows before "+n" (DT-03). */
export const visibleGenreCount = 3;

/**
 * The library route search of a genre link: the Browse pivot filtered by that genre. The key is
 * the `genre` parameter of `listLibraryItems`, which the Browse URL mirrors (M5-31, M5-32).
 */
export interface GenreBrowseSearch {
  readonly view: "browse";
  readonly genre: readonly string[];
}

/** Search of the Browse pivot listing only items of `genre`. */
export function genreBrowseSearch(genre: string): GenreBrowseSearch {
  return { view: "browse", genre: [genre] };
}

/** The genres to show: the first {@link visibleGenreCount}, or all once expanded. */
export function shownGenres(genres: readonly string[], expanded: boolean): readonly string[] {
  return expanded ? genres : genres.slice(0, visibleGenreCount);
}
