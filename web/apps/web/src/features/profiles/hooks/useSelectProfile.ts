import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";

import { useApiClient } from "../../../shared/api";
import { authHref, profilesPath, storeProfileChange } from "../../auth";
import { selectProfile, type ProfileChoice } from "../api/profilesApi";
import { selectionFailure, type SelectionFailure } from "../lib/selectionFailure";

/** A profile choice plus where to go afterwards. */
export interface SelectionRequest {
  readonly choice: ProfileChoice;
  /** A page other than the picker's `next`, e.g. the profile editor after "Add profile". */
  readonly next?: string | undefined;
}

/** Receives why a selection failed. */
export type SelectionFailureHandler = (failure: SelectionFailure) => void;

/** The selection action of the picker. */
export interface ProfileSelection {
  /** Starts a selection; `onFailure` hears about its failure. */
  readonly select: (request: SelectionRequest, onFailure: SelectionFailureHandler) => void;
  readonly pending: boolean;
}

/**
 * Selects a profile. Success stores the account and drops the previous catalog; the session
 * guard then leaves the picker for `next` or Home, so every page loads with the new profile.
 */
export function useSelectProfile(): ProfileSelection {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const mutation = useMutation({
    mutationFn: ({ choice }: SelectionRequest) => selectProfile(client, choice),
    // The variables hold the PIN: with `gcTime: 0` they leave the mutation cache at once.
    gcTime: 0,
    onSuccess: async (me, { next }) => {
      if (next !== undefined) {
        // The guard reads `next` from the URL, so a different target goes there first.
        await navigate({ href: authHref(profilesPath, { next }), replace: true });
      }
      storeProfileChange(queryClient, me);
    },
  });
  return {
    select: (request, onFailure) => {
      if (!mutation.isPending) {
        mutation.mutate(request, {
          onError: (error) => {
            onFailure(selectionFailure(error));
          },
        });
      }
    },
    pending: mutation.isPending,
  };
}
