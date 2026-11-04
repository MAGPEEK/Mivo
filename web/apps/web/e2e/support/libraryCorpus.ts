// The library corpus (scripts/gen-testmedia `-only "library-*"`, written to testdata/library):
// a movies and a shows tree plus the manifest that lists what a scan has to find. Only the
// parts of manifest.json the E2E specs read are typed here.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { repositoryRoot } from "./commands";

/** Directory gen-testmedia writes the library corpus to. */
export const libraryCorpusDirectory = join(repositoryRoot, "testdata", "library");
const manifestPath = join(libraryCorpusDirectory, "manifest.json");

/** Shown by the spec when the corpus has not been generated. */
export const missingLibraryCorpusReason =
  `No library corpus at ${manifestPath}. Generate it with ` +
  '`go run ./scripts/gen-testmedia -only "library-*"` (needs FFmpeg).';

/** One artwork candidate of an expected item. */
export interface ExpectedArtwork {
  readonly kind: string;
  readonly source: string;
}

/** One episode of an expected season. */
export interface ExpectedEpisode {
  readonly number: number;
}

/** One season of an expected show; season 0 holds the specials. */
export interface ExpectedSeason {
  readonly number: number;
  readonly episodes: readonly ExpectedEpisode[];
}

/** One top-level movie or show a scan has to produce. */
export interface ExpectedItem {
  readonly kind: "movie" | "show";
  /** Catalog title after the metadata merge (NFO before file name). */
  readonly title: string;
  readonly artwork?: readonly ExpectedArtwork[];
  readonly seasons?: readonly ExpectedSeason[];
}

/** One library root of the corpus; `path` is its folder below {@link libraryCorpusDirectory}. */
export interface ExpectedLibrary {
  readonly kind: "movies" | "shows";
  readonly path: string;
  readonly items: readonly ExpectedItem[];
}

interface LibraryManifest {
  readonly libraries: readonly ExpectedLibrary[];
}

/** Whether the manifest and both library trees exist, i.e. the corpus has been generated. */
export function libraryCorpusExists(): boolean {
  return [
    manifestPath,
    join(libraryCorpusDirectory, "movies"),
    join(libraryCorpusDirectory, "shows"),
  ].every((path) => existsSync(path));
}

/** The expected library of `kind` from the manifest. */
export function expectedLibrary(kind: ExpectedLibrary["kind"]): ExpectedLibrary {
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as LibraryManifest;
  const found = manifest.libraries.find((library) => library.kind === kind);
  if (found === undefined) {
    throw new Error(`The library corpus manifest has no ${kind} library.`);
  }
  return found;
}

/** Folder of the library corpus tree for `library`, ready to be copied. */
export function libraryCorpusFolder(library: ExpectedLibrary): string {
  return join(libraryCorpusDirectory, library.path);
}
