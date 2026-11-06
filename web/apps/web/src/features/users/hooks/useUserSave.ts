import type { Library, User } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useApiClient } from "../../../shared/api";
import { createUser, resetUserPassword, updateUser, usersQueryKey } from "../api/usersApi";
import {
  draftProblems,
  maxNameLength,
  maxPasswordBytes,
  minPasswordBytes,
  toCreateBody,
  toUpdateBody,
  userDraft,
  type DraftProblems,
  type UserDraft,
} from "../lib/userDraft";
import { userProblem, type ProblemPlace, type UserProblem } from "../lib/userProblems";
import { usersNamespace } from "../locales";

/** Translated messages of the dialog, by the place they show. */
export type UserErrors = Partial<Record<ProblemPlace, string>>;

/** Saving the user dialog. */
export interface UserSave {
  readonly errors: UserErrors;
  readonly pending: boolean;
  readonly save: (draft: UserDraft) => void;
}

const localMessageKeys = {
  name: {
    required: "users.validation.nameRequired",
    tooLong: "users.validation.nameTooLong",
  },
  password: {
    required: "users.validation.passwordRequired",
    tooLong: "users.validation.passwordTooLong",
    tooShort: "users.validation.passwordTooShort",
  },
} as const;

function useErrorTexts(local: DraftProblems, server: UserProblem | null): UserErrors {
  const { t } = useTranslation(usersNamespace);
  const errors: UserErrors = {};
  if (local.name !== undefined) {
    errors.name = t(localMessageKeys.name[local.name], { max: maxNameLength });
  }
  if (local.password !== undefined) {
    errors.password = t(localMessageKeys.password[local.password], {
      min: minPasswordBytes,
      max: maxPasswordBytes,
    });
  }
  if (server !== null) {
    errors[server.place] = t(`users.problems.${server.message}`);
  }
  return errors;
}

interface SaveVariables {
  readonly draft: UserDraft;
  readonly libraryIds: readonly string[];
}

/**
 * Creates the account, or updates `user` when given: first the account changes, then the new
 * password when "Reset password" was used. On success the user list is refreshed and
 * `onSaved` runs.
 */
export function useUserSave(
  user: User | undefined,
  libraries: readonly Library[],
  onSaved: () => void,
): UserSave {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [local, setLocal] = useState<DraftProblems>({});
  const mutation = useMutation<unknown, unknown, SaveVariables>({
    mutationFn: async ({ draft, libraryIds }) => {
      if (user === undefined) {
        return createUser(client, toCreateBody(draft, libraryIds));
      }
      const body = toUpdateBody(draft, userDraft(user), libraryIds);
      if (Object.keys(body).length > 0) {
        await updateUser(client, { userId: user.id, body });
      }
      if (draft.password !== "") {
        await resetUserPassword(client, { userId: user.id, password: draft.password });
      }
      return undefined;
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
    onSuccess: onSaved,
  });
  const server = mutation.error === null ? null : userProblem(mutation.error);
  const errors = useErrorTexts(local, server);
  return {
    errors,
    pending: mutation.isPending,
    save: (draft) => {
      const problems = draftProblems(draft, user === undefined);
      setLocal(problems);
      mutation.reset();
      if (Object.keys(problems).length === 0) {
        mutation.mutate({ draft, libraryIds: libraries.map((library) => library.id) });
      }
    },
  };
}
