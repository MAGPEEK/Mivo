import type { Library, LibraryAccess } from "@mivo/api-client";

import type { LibraryAccessDraft } from "../../lib/libraryAccess";

/** A library in the editor: `allowed` when the account itself may see it (ADR 0032 §7). */
export interface LibraryChoice {
  readonly library: Library;
  readonly allowed: boolean;
}

/**
 * Marks which of `libraries` a profile of the account can get: a managed profile sees a
 * subset of its account's libraries, never more.
 */
export function libraryChoices(
  libraries: readonly Library[],
  accountAccess: LibraryAccess,
): LibraryChoice[] {
  const accountIds = accountAccess.libraryIds ?? [];
  return libraries.map((library) => ({
    library,
    allowed: accountAccess.all || accountIds.includes(library.id),
  }));
}

/** Whether the draft has `choice` ticked; a library outside the account's set never is. */
export function isChosen(draft: LibraryAccessDraft, choice: LibraryChoice): boolean {
  return choice.allowed && draft.libraryIds.includes(choice.library.id);
}

/**
 * The request body of a draft. "All" means the account's set, also for libraries added
 * later; a selection keeps only libraries that exist and that the account may see.
 */
export function toProfileAccess(
  draft: LibraryAccessDraft,
  choices: readonly LibraryChoice[],
): LibraryAccess {
  if (draft.all) {
    return { all: true };
  }
  const libraryIds = choices
    .filter((choice) => isChosen(draft, choice))
    .map((choice) => choice.library.id);
  return { all: false, libraryIds };
}
