import type { Library } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { librariesQueryKey } from "../../library";
import { useApiClient } from "../../../shared/api";
import { deleteLibrary } from "../api/libraryManagement";
import { useManagementErrorText } from "./useManagementErrorText";

/** State and actions of the "Remove library" confirmation. */
export interface LibraryRemoval {
  /** The library the dialog asks about; null while the dialog is closed. */
  readonly target: Library | null;
  readonly pending: boolean;
  /** Why the last removal failed, for display inside the dialog; null otherwise. */
  readonly errorText: string | null;
  /** Opens the confirmation for `library`. */
  readonly ask: (library: Library) => void;
  readonly cancel: () => void;
  readonly confirm: () => void;
}

/**
 * Drives removing a library: confirmation first, then `deleteLibrary`. On success every
 * library query is invalidated (sidebar, Home and this list refresh) and `onRemoved` runs.
 */
export function useLibraryRemoval(onRemoved: () => void): LibraryRemoval {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<Library | null>(null);
  const mutation = useMutation<unknown, Error, string>({
    mutationFn: (libraryId) => deleteLibrary(client, libraryId),
    onSuccess: async () => {
      setTarget(null);
      await queryClient.invalidateQueries({ queryKey: librariesQueryKey });
      onRemoved();
    },
  });
  const { reset, mutate } = mutation;
  const errorText = useManagementErrorText(mutation.error, "libraries.remove.failed");
  return {
    target,
    pending: mutation.isPending,
    errorText,
    ask: (library) => {
      reset();
      setTarget(library);
    },
    cancel: () => {
      // Closing mid-request would hide the outcome (e.g. a 403) of a delete that still runs.
      if (!mutation.isPending) {
        reset();
        setTarget(null);
      }
    },
    confirm: () => {
      if (target !== null) {
        mutate(target.id);
      }
    },
  };
}
