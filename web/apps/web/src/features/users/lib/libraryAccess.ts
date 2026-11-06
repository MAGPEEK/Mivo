import type { LibraryAccess } from "@mivo/api-client";

/** Library access as the dialog edits it (US-22). */
export interface LibraryAccessDraft {
  /** True for every library, including those added later. */
  readonly all: boolean;
  /** The chosen libraries; kept while "All" is on, so switching back restores the choice. */
  readonly libraryIds: readonly string[];
}

/** Every library: the default of a new account. */
export const allLibraries: LibraryAccessDraft = { all: true, libraryIds: [] };

/** The draft of stored access. */
export function libraryAccessDraft(access: LibraryAccess): LibraryAccessDraft {
  return { all: access.all, libraryIds: access.libraryIds ?? [] };
}

/** Adds or removes one library from the selection. */
export function withLibrary(
  draft: LibraryAccessDraft,
  libraryId: string,
  selected: boolean,
): LibraryAccessDraft {
  const others = draft.libraryIds.filter((id) => id !== libraryId);
  return { ...draft, libraryIds: selected ? [...others, libraryId] : others };
}

/**
 * The request body of a draft. Only IDs of libraries that still exist go out: a library
 * removed meanwhile would make the server answer 400 "invalid".
 */
export function toLibraryAccess(
  draft: LibraryAccessDraft,
  knownLibraryIds: readonly string[],
): LibraryAccess {
  if (draft.all) {
    return { all: true };
  }
  return { all: false, libraryIds: draft.libraryIds.filter((id) => knownLibraryIds.includes(id)) };
}

/** Whether two drafts grant the same libraries. */
export function sameLibraryAccess(left: LibraryAccessDraft, right: LibraryAccessDraft): boolean {
  if (left.all || right.all) {
    return left.all === right.all;
  }
  return (
    left.libraryIds.length === right.libraryIds.length &&
    left.libraryIds.every((id) => right.libraryIds.includes(id))
  );
}
