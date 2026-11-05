import {
  ApiError,
  type FolderRef,
  type FolderRoot,
  type Library,
  type LibraryCreate,
  type LibraryKind,
  type LibraryOptions,
  type LibraryUpdate,
} from "@mivo/api-client";

/** A folder in the dialog: either kept from the library (`folderId`) or newly chosen. */
export interface DraftFolder {
  /** Stable React key; unique within the draft. */
  readonly key: string;
  /** Absolute path for display. */
  readonly path: string;
  readonly ref: FolderRef;
}

/** Everything the user edits in the library dialog. */
export interface LibraryDraft {
  readonly kind: LibraryKind;
  readonly name: string;
  /** Whether the user typed the name; until then it follows the first folder. */
  readonly nameTouched: boolean;
  /** BCP 47 tag; empty selects the server default. */
  readonly language: string;
  readonly folders: readonly DraftFolder[];
  readonly options: LibraryOptions;
}

/** Folder limits of the API. */
export const maxFolders = 16;

/** Languages offered for metadata; empty is the server default. */
export const metadataLanguageCodes: readonly string[] = [
  "",
  "en",
  "de",
  "fr",
  "es",
  "it",
  "nl",
  "pl",
  "pt",
  "pt-BR",
  "sv",
  "da",
  "no",
  "fi",
  "cs",
  "ja",
  "ko",
  "zh",
];

/** Options of a new library: everything the scanner supports is on. */
export const defaultOptions: LibraryOptions = {
  scanExtras: true,
  readNfo: true,
  localArtwork: true,
  onlineMetadata: true,
};

const pathSeparators = /[\\/]+/;

/** Last non-empty element of a path; separators of either kind count. Empty for no elements. */
export function lastPathSegment(path: string): string {
  return (
    path
      .split(pathSeparators)
      .filter((segment) => segment !== "")
      .pop() ?? ""
  );
}

/** Absolute display path of `relative` below `rootPath`. */
export function joinDisplayPath(rootPath: string, relative: string): string {
  if (relative === "") {
    return rootPath;
  }
  const separator = rootPath.includes("\\") && !rootPath.includes("/") ? "\\" : "/";
  const base = rootPath.endsWith("/") || rootPath.endsWith("\\") ? rootPath.slice(0, -1) : rootPath;
  return `${base}${separator}${relative.split("/").join(separator)}`;
}

/** The draft folder for a folder the library already has. */
export function keptFolder(folder: Library["folders"][number]): DraftFolder {
  return { key: folder.id, path: folder.path, ref: { folderId: folder.id } };
}

/** The draft folder for a folder just chosen in the browser. */
export function chosenFolder(root: FolderRoot, relative: string, key: string): DraftFolder {
  return {
    key,
    path: joinDisplayPath(root.path, relative),
    ref: { rootId: root.id, path: relative },
  };
}

/** Name suggested by the first folder: its last path element (the root's when it is chosen). */
export function suggestedName(folders: readonly DraftFolder[]): string {
  const first = folders[0];
  return first === undefined ? "" : lastPathSegment(first.path);
}

/** The empty draft of "Add library". */
export function emptyDraft(kind: LibraryKind): LibraryDraft {
  return {
    kind,
    name: "",
    nameTouched: false,
    language: "",
    folders: [],
    options: defaultOptions,
  };
}

/** The draft of "Edit library", seeded from `library`. */
export function draftOf(library: Library): LibraryDraft {
  return {
    kind: library.kind,
    name: library.name,
    nameTouched: true,
    language: library.metadataLanguage,
    folders: library.folders.map(keptFolder),
    options: library.options,
  };
}

/** Whether two drafts would save the same library. */
export function sameDraft(a: LibraryDraft, b: LibraryDraft): boolean {
  return (
    a.kind === b.kind &&
    a.name.trim() === b.name.trim() &&
    a.language === b.language &&
    a.options.scanExtras === b.options.scanExtras &&
    a.options.readNfo === b.options.readNfo &&
    a.options.localArtwork === b.options.localArtwork &&
    (a.options.onlineMetadata ?? true) === (b.options.onlineMetadata ?? true) &&
    a.folders.length === b.folders.length &&
    a.folders.every((folder, index) => folder.key === b.folders[index]?.key)
  );
}

/** Body of `createLibrary` for `draft`. */
export function createBody(draft: LibraryDraft): LibraryCreate {
  return {
    kind: draft.kind,
    name: draft.name.trim(),
    folders: draft.folders.map((folder) => folder.ref),
    metadataLanguage: draft.language,
    options: draft.options,
  };
}

/** Body of `updateLibrary` for `draft`; the kind is not part of it. */
export function updateBody(draft: LibraryDraft): LibraryUpdate {
  return {
    name: draft.name.trim(),
    folders: draft.folders.map((folder) => folder.ref),
    metadataLanguage: draft.language,
    options: draft.options,
  };
}

/** Which part of the dialog a failed save belongs to. */
export type ProblemScope = "name" | "language" | "kind" | "folders" | "folder" | "general";

/** A failed save, sorted to the field it is about. */
export interface SaveProblem {
  readonly scope: ProblemScope;
  /** Server problem code; `unknown` for anything that is not a problem document. */
  readonly code: string;
  /** Index into the folder list for scope `folder`. */
  readonly folderIndex?: number;
  /** For `folder_overlap`: the library that holds the other folder. */
  readonly libraryId?: string | undefined;
  /** English server text, used for codes the client has no translation for. */
  readonly reason: string;
}

const folderFieldPattern = /^folders\[(\d+)\]$/;

const scopeByCode: Readonly<Record<string, ProblemScope>> = {
  name_invalid: "name",
  conflict: "name",
  language_invalid: "language",
  kind_unsupported: "kind",
  too_many_folders: "folders",
  folder_not_found: "folder",
  folder_unreadable: "folder",
  folder_overlap: "folder",
  folder_reserved: "folder",
  folder_outside_roots: "folder",
};

function folderIndexOf(field: string | undefined): number | undefined {
  const match = field === undefined ? null : folderFieldPattern.exec(field);
  return match?.[1] === undefined ? undefined : Number(match[1]);
}

/** Sorts a failed `createLibrary` / `updateLibrary` call to the field it is about. */
export function classifyProblem(error: Error): SaveProblem {
  if (!(error instanceof ApiError)) {
    return { scope: "general", code: "unknown", reason: error.message };
  }
  const reason = error.detail ?? error.title;
  const scope = scopeByCode[error.code] ?? "general";
  if (scope === "folder") {
    const folderIndex = folderIndexOf(error.field);
    // Without an index the problem concerns the folder list as a whole.
    return folderIndex === undefined
      ? { scope: "folders", code: error.code, libraryId: error.libraryId, reason }
      : { scope, code: error.code, folderIndex, libraryId: error.libraryId, reason };
  }
  return { scope, code: error.code, reason };
}
