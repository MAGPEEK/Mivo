// Folder paths of the folder view (task M5-36). They come from the URL, so they are untrusted:
// the checks here only spare the server requests that can never succeed. The server stays
// authoritative and answers 400 or 404 for anything it does not accept.

/** The longest path `listLibraryFolders` accepts. */
const maxPathLength = 4096;
const pathSeparator = "/";
// ASCII control characters (NUL, line breaks...) never belong to a path from the API.
// eslint-disable-next-line no-control-regex -- matching control characters is the point
const controlCharacters = /[\u0000-\u001f\u007f]/;
const windowsDrive = /^[A-Za-z]:$/;
const dotSegments: readonly string[] = [".", ".."];

/** A folder path from the URL: valid with its segments, or rejected. */
export type FolderPathResult =
  | {
      readonly valid: true;
      /** Library-relative path joined with "/", without a trailing slash; empty at the top. */
      readonly path: string;
      /** The names below the library; the first one is a LibraryFolder id. */
      readonly segments: readonly string[];
    }
  | { readonly valid: false };

const invalid: FolderPathResult = { valid: false };

function isSafeSegment(segment: string, index: number): boolean {
  return (
    segment !== "" &&
    !dotSegments.includes(segment) &&
    !segment.includes("\\") &&
    !controlCharacters.test(segment) &&
    // A drive letter first would be an absolute Windows path; deeper it is just a name.
    !(index === 0 && windowsDrive.test(segment))
  );
}

/**
 * Validates the splat of `/libraries/$libraryId/folders/*`: no `.` or `..` segment, no absolute
 * path, no empty segment, no backslash. One trailing slash is dropped. Absent or empty is the top.
 */
export function parseFolderPath(splat: string | undefined): FolderPathResult {
  const trimmed = splat?.endsWith(pathSeparator) === true ? splat.slice(0, -1) : (splat ?? "");
  if (trimmed === "") {
    return { valid: true, path: "", segments: [] };
  }
  if (trimmed.length > maxPathLength) {
    return invalid;
  }
  const segments = trimmed.split(pathSeparator);
  return segments.every((segment, index) => isSafeSegment(segment, index))
    ? { valid: true, path: trimmed, segments }
    : invalid;
}

/**
 * The last name of a server path, either separator. A library folder is shown by this name only,
 * never by its absolute path.
 */
export function folderBasename(path: string): string {
  const names = path.split(/[\\/]/).filter((name) => name !== "");
  return names.at(-1) ?? path;
}

/** One link of the breadcrumb: a folder name and the library-relative path it opens. */
export interface FolderCrumb {
  readonly name: string;
  readonly path: string;
}

/**
 * The breadcrumb below the library root: one crumb per segment. The first segment is a folder id,
 * so it is shown as `topFolderName`; the others are names already.
 */
export function folderCrumbs(
  segments: readonly string[],
  topFolderName: string,
): readonly FolderCrumb[] {
  return segments.map((segment, index) => ({
    name: index === 0 ? topFolderName : segment,
    path: segments.slice(0, index + 1).join(pathSeparator),
  }));
}
