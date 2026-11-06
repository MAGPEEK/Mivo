import type { User } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { deleteUser, usersQueryKey } from "../api/usersApi";
import { userProblem } from "../lib/userProblems";
import { usersNamespace } from "../locales";

/** State and actions of the "Delete account" confirmation. */
export interface UserRemoval {
  /** The account the dialog asks about; null while the dialog is closed. */
  readonly target: User | null;
  readonly pending: boolean;
  /** Why the last deletion failed, e.g. the last admin; null otherwise. */
  readonly errorText: string | null;
  readonly ask: (user: User) => void;
  readonly cancel: () => void;
  readonly confirm: () => void;
}

/**
 * Drives deleting an account: confirmation first, then `deleteUser`. On success the user list
 * is refreshed and `onRemoved` runs.
 */
export function useUserRemoval(onRemoved: () => void): UserRemoval {
  const { t } = useTranslation(usersNamespace);
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<User | null>(null);
  const mutation = useMutation<unknown, unknown, string>({
    mutationFn: (userId) => deleteUser(client, userId),
    onSuccess: async () => {
      setTarget(null);
      await queryClient.invalidateQueries({ queryKey: usersQueryKey });
      onRemoved();
    },
  });
  const { reset, mutate } = mutation;
  const errorText =
    mutation.error === null ? null : t(`users.problems.${userProblem(mutation.error).message}`);
  return {
    target,
    pending: mutation.isPending,
    errorText,
    ask: (user) => {
      reset();
      setTarget(user);
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
