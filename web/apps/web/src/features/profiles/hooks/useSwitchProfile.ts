import type { Me } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { storeProfileChange } from "../../auth";
import { clearProfileSelection } from "../api/profilesApi";

/** The "Switch profile" action of the profile menu (SH-07). */
export interface SwitchProfileAction {
  readonly switchProfile: () => void;
  readonly failed: boolean;
}

/**
 * Leaves the selected profile. The session stays signed in; with no profile selected the guard
 * opens the picker and passes the current page on as `next`.
 */
export function useSwitchProfile(me: Me): SwitchProfileAction {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => clearProfileSelection(client),
    onSuccess: () => {
      storeProfileChange(queryClient, { ...me, currentProfile: null });
    },
  });
  return {
    switchProfile: () => {
      if (!mutation.isPending) {
        mutation.mutate();
      }
    },
    failed: mutation.isError,
  };
}
