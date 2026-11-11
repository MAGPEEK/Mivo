import type { Library } from "@mivo/api-client";

import { useLibraries } from "../../library";

/** How far the libraries of an access choice are; a draft is saved only once they are `ready`. */
export type LibraryLoadStatus = "loading" | "failed" | "ready";

/** The libraries an access choice offers, with their load state. */
export interface LibraryList {
  readonly status: LibraryLoadStatus;
  /** Empty until `ready`. */
  readonly libraries: readonly Library[];
  /** Loads the libraries again after a failure. */
  readonly retry: () => void;
}

const noLibraries: readonly Library[] = [];

function loadStatus(loaded: boolean, failed: boolean): LibraryLoadStatus {
  if (loaded) {
    return "ready";
  }
  return failed ? "failed" : "loading";
}

/**
 * Loads the libraries for the access choice of an account or profile. "Not loaded" stays
 * apart from "there are none": a save that took a list still loading for an empty one would
 * send `libraryIds: []` and take every library from the user.
 */
export function useLibraryList(): LibraryList {
  const query = useLibraries();
  return {
    status: loadStatus(query.data !== undefined, query.isError),
    libraries: query.data ?? noLibraries,
    retry: () => void query.refetch(),
  };
}
