import type { LibraryKind, LibraryOptions } from "@mivo/api-client";
import { useRef, useState } from "react";

import { sameDraft, suggestedName, type DraftFolder, type LibraryDraft } from "../lib/libraryDraft";

/** The editable state of the library dialog and the actions that change it. */
export interface LibraryForm {
  readonly draft: LibraryDraft;
  /** Whether the draft differs from what the dialog opened with. */
  readonly dirty: boolean;
  readonly setKind: (kind: LibraryKind) => void;
  readonly setName: (name: string) => void;
  readonly setLanguage: (language: string) => void;
  readonly setOption: (option: keyof LibraryOptions, enabled: boolean) => void;
  /** Adds a folder; `create` receives a key that is unique within the draft. */
  readonly addFolder: (create: (key: string) => DraftFolder) => void;
  readonly removeFolder: (key: string) => void;
}

// While the user has not typed a name it follows the first folder, so a library named after
// its folder needs no typing.
function withFolders(draft: LibraryDraft, folders: readonly DraftFolder[]): LibraryDraft {
  return draft.nameTouched
    ? { ...draft, folders }
    : { ...draft, folders, name: suggestedName(folders) };
}

/**
 * State of the add/edit dialog. `onEdit` runs on every change so the caller can drop a stale
 * save error: a message about the old input must not stay next to the new one.
 */
export function useLibraryForm(initial: LibraryDraft, onEdit: () => void): LibraryForm {
  const [draft, setDraft] = useState(initial);
  const [original] = useState(initial);
  const nextKey = useRef(0);
  const update = (change: (current: LibraryDraft) => LibraryDraft) => {
    onEdit();
    setDraft(change);
  };
  return {
    draft,
    dirty: !sameDraft(draft, original),
    setKind: (kind) => {
      update((current) => ({ ...current, kind }));
    },
    setName: (name) => {
      update((current) => ({ ...current, name, nameTouched: true }));
    },
    setLanguage: (language) => {
      update((current) => ({ ...current, language }));
    },
    setOption: (option, enabled) => {
      update((current) => ({ ...current, options: { ...current.options, [option]: enabled } }));
    },
    addFolder: (create) => {
      nextKey.current += 1;
      const folder = create(`new-${String(nextKey.current)}`);
      update((current) => withFolders(current, [...current.folders, folder]));
    },
    removeFolder: (key) => {
      update((current) =>
        withFolders(
          current,
          current.folders.filter((folder) => folder.key !== key),
        ),
      );
    },
  };
}
