import type { Profile } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../../shared/api";
import { refreshAccounts } from "../../api/usersApi";
import { maxNameLength } from "../../lib/userDraft";
import { usersNamespace } from "../../locales";
import { createProfile, updateProfile } from "../api/managedProfilesApi";
import { pinLength } from "../lib/pin";
import type { LibraryChoice } from "../lib/profileAccess";
import {
  maxManagedProfiles,
  profileDraftProblems,
  toProfileCreateBody,
  toProfileUpdateBody,
  type ProfileDraft,
  type ProfileDraftProblems,
} from "../lib/profileDraft";
import {
  profileProblem,
  type ProfileProblem,
  type ProfileProblemPlace,
} from "../lib/profileProblems";

/** Translated messages of the editor, by the place they show. */
export type ProfileErrors = Partial<Record<ProfileProblemPlace, string>>;

/** Saving the profile editor. */
export interface ProfileSave {
  readonly errors: ProfileErrors;
  readonly pending: boolean;
  /** Checks the draft and saves it. `choices` are the libraries that exist now. */
  readonly save: (draft: ProfileDraft, choices: readonly LibraryChoice[]) => void;
}

/** What {@link useProfileSave} saves: a new profile of `ownerId`, or changes to `profile`. */
export interface ProfileSaveTarget {
  readonly ownerId: string;
  readonly profile: Profile | undefined;
}

const nameMessageKeys = {
  required: "users.validation.nameRequired",
  tooLong: "users.validation.nameTooLong",
} as const;

const pinMessageKeys = {
  required: "profiles.validation.pinRequired",
  invalid: "profiles.validation.pinInvalid",
} as const;

function useErrorTexts(local: ProfileDraftProblems, server: ProfileProblem | null): ProfileErrors {
  const { t } = useTranslation(usersNamespace);
  const errors: ProfileErrors = {};
  if (local.name !== undefined) {
    errors.name = t(nameMessageKeys[local.name], { max: maxNameLength });
  }
  if (local.pin !== undefined) {
    errors.pin = t(pinMessageKeys[local.pin], { length: pinLength });
  }
  if (server !== null) {
    errors[server.place] = t(`profiles.problems.${server.message}`, { max: maxManagedProfiles });
  }
  return errors;
}

interface SaveVariables {
  readonly draft: ProfileDraft;
  readonly choices: readonly LibraryChoice[];
}

/**
 * Creates the managed profile, or updates the given one with only what changed. Afterwards
 * the user administration and the signed-in account load again, and on success `onSaved` runs.
 */
export function useProfileSave(target: ProfileSaveTarget, onSaved: () => void): ProfileSave {
  const { ownerId, profile } = target;
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [local, setLocal] = useState<ProfileDraftProblems>({});
  const mutation = useMutation<unknown, unknown, SaveVariables>({
    mutationFn: async ({ draft, choices }) => {
      if (profile === undefined) {
        return createProfile(client, {
          userId: ownerId,
          body: toProfileCreateBody(draft, choices),
        });
      }
      const body = toProfileUpdateBody(draft, profile, choices);
      return Object.keys(body).length > 0
        ? updateProfile(client, { profileId: profile.id, body })
        : undefined;
    },
    // The variables hold the PIN: with `gcTime: 0` they leave the mutation cache at once.
    gcTime: 0,
    onSettled: () => refreshAccounts(queryClient),
    onSuccess: onSaved,
  });
  const server = mutation.error === null ? null : profileProblem(mutation.error);
  return {
    errors: useErrorTexts(local, server),
    pending: mutation.isPending,
    save: (draft, choices) => {
      const problems = profileDraftProblems(draft, profile);
      setLocal(problems);
      mutation.reset();
      if (Object.keys(problems).length === 0) {
        mutation.mutate({ draft, choices });
      }
    },
  };
}
