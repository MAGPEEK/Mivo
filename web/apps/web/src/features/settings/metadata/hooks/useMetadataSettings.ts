import { ApiError } from "@mivo/api-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useApiClient } from "../../../../shared/api";
import {
  isNotImplemented,
  metadataSettingsQueryKey,
  metadataSettingsQueryOptions,
  updateMetadataSettings,
  type MetadataSettings,
  type MetadataSettingsUpdate,
} from "../api/metadataSettings";

/** What the settings query shows. */
export type MetadataSettingsLoad =
  | { readonly status: "loading" }
  /** The server does not serve online metadata yet (501). */
  | { readonly status: "unavailable" }
  | { readonly status: "error"; readonly retry: () => void }
  | { readonly status: "ready"; readonly settings: MetadataSettings };

/** Why the last change failed: a known problem code, or "failed" for anything else. */
export type MetadataSaveProblem = "language_invalid" | "forbidden" | "unavailable" | "failed";

/** The settings, the state of the last change and the action to change them. */
export interface MetadataSettingsControls {
  readonly load: MetadataSettingsLoad;
  /** Whether a change is being saved. */
  readonly pending: boolean;
  /** Why the last change failed; null after a success or before any change. */
  readonly problem: MetadataSaveProblem | null;
  /** Saves `changes`; the answer replaces the cached settings. */
  readonly update: (changes: MetadataSettingsChanges) => void;
}

/** Changes of the switches and the language; the TMDB key goes through `useTmdbKeyField`. */
export type MetadataSettingsChanges = Omit<MetadataSettingsUpdate, "tmdbApiKey">;

const forbiddenStatus = 403;

/** Sorts a failed change into the problem the page shows. */
export function problemOf(error: Error): MetadataSaveProblem {
  if (isNotImplemented(error)) {
    return "unavailable";
  }
  if (error instanceof ApiError && error.code === "language_invalid") {
    return "language_invalid";
  }
  return error instanceof ApiError && error.status === forbiddenStatus ? "forbidden" : "failed";
}

/**
 * Loads the online metadata settings and changes them. The answer of a change replaces the
 * cached settings, so the page shows what the server stored. The TMDB key is not changed here:
 * it has its own mutation in `useTmdbKeyField` that drops it from React Query state.
 */
export function useMetadataSettings(): MetadataSettingsControls {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const query = useQuery(metadataSettingsQueryOptions(client));
  const mutation = useMutation<MetadataSettings, Error, MetadataSettingsChanges>({
    mutationFn: (changes) => updateMetadataSettings(client, changes),
    onSuccess: (saved) => {
      queryClient.setQueryData(metadataSettingsQueryKey, saved);
    },
  });
  return {
    load: loadOf(query),
    pending: mutation.isPending,
    problem: mutation.error === null ? null : problemOf(mutation.error),
    update: (changes) => {
      mutation.mutate(changes);
    },
  };
}

interface SettingsQuery {
  readonly data: MetadataSettings | undefined;
  readonly error: Error | null;
  readonly refetch: () => unknown;
}

function loadOf(query: SettingsQuery): MetadataSettingsLoad {
  if (query.data !== undefined) {
    return { status: "ready", settings: query.data };
  }
  if (query.error === null) {
    return { status: "loading" };
  }
  if (isNotImplemented(query.error)) {
    return { status: "unavailable" };
  }
  return {
    status: "error",
    retry: () => {
      void query.refetch();
    },
  };
}
