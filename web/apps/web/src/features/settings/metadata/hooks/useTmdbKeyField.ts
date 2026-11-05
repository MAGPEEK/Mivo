import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApiClient } from "../../../../shared/api";
import {
  metadataSettingsQueryKey,
  updateMetadataSettings,
  type MetadataSettings,
} from "../api/metadataSettings";
import { problemOf, type MetadataSaveProblem } from "./useMetadataSettings";

/** The TMDB key input: its draft and the actions to save or remove the own key. */
export interface TmdbKeyField {
  /** What the user typed; cleared once the server stored it. */
  readonly draft: string;
  readonly setDraft: (value: string) => void;
  /** Whether a key change is being saved. */
  readonly pending: boolean;
  /** Whether "Save key" may run: something typed and nothing pending. */
  readonly canSave: boolean;
  /** Why the last key change failed; null after a success or before any change. */
  readonly problem: MetadataSaveProblem | null;
  readonly save: () => void;
  /** Removes the own key, so the project key applies again. */
  readonly remove: () => void;
}

/**
 * State of the write-only TMDB key field. The key has its own mutation, so toggles saved in
 * parallel never touch the draft. The mutation is reset as soon as it settles and is never
 * cached (`gcTime: 0`), so the key does not stay in React Query state; the server never
 * returns it, and the field empties after saving.
 */
export function useTmdbKeyField(): TmdbKeyField {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState("");
  const [problem, setProblem] = useState<MetadataSaveProblem | null>(null);
  const mutation = useMutation<MetadataSettings, Error, string>({
    mutationFn: (tmdbApiKey) => updateMetadataSettings(client, { tmdbApiKey }),
    gcTime: 0,
  });
  const { mutate, reset } = mutation;
  const trimmed = draft.trim();
  const send = (tmdbApiKey: string) => {
    setProblem(null);
    mutate(tmdbApiKey, {
      onSuccess: (saved) => {
        queryClient.setQueryData(metadataSettingsQueryKey, saved);
        setDraft("");
      },
      onError: (error) => {
        setProblem(problemOf(error));
      },
      onSettled: () => {
        reset();
      },
    });
  };
  return {
    draft,
    setDraft,
    pending: mutation.isPending,
    canSave: trimmed !== "" && !mutation.isPending,
    problem,
    save: () => {
      if (trimmed !== "") {
        send(trimmed);
      }
    },
    remove: () => {
      send("");
    },
  };
}
