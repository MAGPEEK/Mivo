import type { Library } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApiClient } from "../../../shared/api";
import { librariesQueryKey } from "../../library";
import { createLibrary, updateLibrary } from "../api/libraryManagement";
import {
  classifyProblem,
  createBody,
  maxFolders,
  updateBody,
  type LibraryDraft,
  type SaveProblem,
} from "../lib/libraryDraft";

/** Saving the dialog: what is wrong, whether a request runs, and the action. */
export interface LibrarySave {
  /** The first problem to show: a local check or the server's answer; null when none. */
  readonly problem: SaveProblem | null;
  readonly pending: boolean;
  readonly save: (draft: LibraryDraft) => void;
  /** Forgets the current problem, e.g. because the input changed. */
  readonly clearProblem: () => void;
}

const noFoldersProblem: SaveProblem = { scope: "folders", code: "folders_required", reason: "" };
const noNameProblem: SaveProblem = { scope: "name", code: "name_required", reason: "" };
const tooManyFoldersProblem: SaveProblem = {
  scope: "folders",
  code: "too_many_folders",
  reason: "",
};

// The server checks everything again; these only spare a round trip for the obvious cases.
function localProblemOf(draft: LibraryDraft): SaveProblem | null {
  if (draft.name.trim() === "") {
    return noNameProblem;
  }
  if (draft.folders.length === 0) {
    return noFoldersProblem;
  }
  return draft.folders.length > maxFolders ? tooManyFoldersProblem : null;
}

/** Callbacks of {@link useLibrarySave}. */
export interface LibrarySaveHandlers {
  /** Runs after the library list was refreshed. */
  readonly onSaved: (saved: Library) => void;
  /** Runs when a local check or the server rejects the draft, so the UI can show the right tab. */
  readonly onProblem: (problem: SaveProblem) => void;
}

/**
 * Creates the library, or updates `library` when given. On success every library query is
 * invalidated (sidebar, Home and the settings list refresh) and `onSaved` gets the saved library.
 */
export function useLibrarySave(
  library: Library | undefined,
  handlers: LibrarySaveHandlers,
): LibrarySave {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [localProblem, setLocalProblem] = useState<SaveProblem | null>(null);
  const mutation = useMutation<Library, Error, LibraryDraft>({
    mutationFn: (draft) =>
      library === undefined
        ? createLibrary(client, createBody(draft))
        : updateLibrary(client, { libraryId: library.id, body: updateBody(draft) }),
    onSuccess: async (saved) => {
      await queryClient.invalidateQueries({ queryKey: librariesQueryKey });
      handlers.onSaved(saved);
    },
    onError: (error) => {
      handlers.onProblem(classifyProblem(error));
    },
  });
  const { mutate, reset } = mutation;
  const serverProblem = mutation.error === null ? null : classifyProblem(mutation.error);
  return {
    problem: localProblem ?? serverProblem,
    pending: mutation.isPending,
    save: (draft) => {
      const found = localProblemOf(draft);
      setLocalProblem(found);
      if (found === null) {
        mutate(draft);
      } else {
        handlers.onProblem(found);
      }
    },
    clearProblem: () => {
      setLocalProblem(null);
      reset();
    },
  };
}
