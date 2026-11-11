import type { Profile } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../../shared/api";
import { refreshAccounts } from "../../api/usersApi";
import { usersNamespace } from "../../locales";
import { deleteProfile } from "../api/managedProfilesApi";
import { maxManagedProfiles } from "../lib/profileDraft";
import { profileProblem } from "../lib/profileProblems";

/** State and actions of the "Delete profile" confirmation. */
export interface ProfileRemoval {
  /** The profile the dialog asks about; null while the dialog is closed. */
  readonly target: Profile | null;
  readonly pending: boolean;
  /** Why the last deletion failed; null otherwise. */
  readonly errorText: string | null;
  readonly ask: (profile: Profile) => void;
  readonly cancel: () => void;
  readonly confirm: () => void;
}

/**
 * Drives deleting a managed profile: confirmation first, then `deleteProfile`. Afterwards the
 * user administration and the signed-in account load again; on success `onRemoved` runs.
 */
export function useProfileRemoval(onRemoved: () => void): ProfileRemoval {
  const { t } = useTranslation(usersNamespace);
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<Profile | null>(null);
  const mutation = useMutation<unknown, unknown, string>({
    mutationFn: (profileId) => deleteProfile(client, profileId),
    // Also after a failure: a "not found" means the list on screen is out of date.
    onSettled: () => refreshAccounts(queryClient),
    onSuccess: () => {
      setTarget(null);
      onRemoved();
    },
  });
  const { reset, mutate } = mutation;
  const errorText =
    mutation.error === null
      ? null
      : t(`profiles.problems.${profileProblem(mutation.error).message}`, {
          max: maxManagedProfiles,
        });
  return {
    target,
    pending: mutation.isPending,
    errorText,
    ask: (profile) => {
      reset();
      setTarget(profile);
    },
    cancel: () => {
      // Closing mid-request would hide the outcome of a delete that still runs.
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
