import type { Library, User } from "@mivo/api-client";
import { useState } from "react";

import { useLibraries } from "../../library";
import { newUserDraft, userDraft } from "../lib/userDraft";
import { useUserDraft, type UserDraftState } from "./useUserDraft";
import { useUserSave, type UserErrors } from "./useUserSave";

/** Everything the create/edit dialog renders and does. */
export interface UserDialogState extends UserDraftState {
  /** True for "Add user", false when editing an account. */
  readonly creating: boolean;
  readonly libraries: readonly Library[];
  readonly errors: UserErrors;
  readonly pending: boolean;
  /** True after "Reset password": the dialog shows the new-password field. */
  readonly resettingPassword: boolean;
  readonly startPasswordReset: () => void;
  readonly cancelPasswordReset: () => void;
  /** Checks and saves the draft; the one Save button of the dialog. */
  readonly submit: () => void;
}

const noLibraries: readonly Library[] = [];

/** State of the dialog that adds an account, or edits `user` when given. */
export function useUserDialog(
  user: User | undefined,
  onSaved: (name: string) => void,
): UserDialogState {
  const state = useUserDraft(user === undefined ? newUserDraft : userDraft(user));
  const libraries = useLibraries().data ?? noLibraries;
  const [resettingPassword, setResettingPassword] = useState(false);
  const save = useUserSave(user, libraries, () => {
    onSaved(state.draft.name.trim());
  });
  return {
    ...state,
    creating: user === undefined,
    libraries,
    errors: save.errors,
    pending: save.pending,
    resettingPassword,
    startPasswordReset: () => {
      setResettingPassword(true);
    },
    cancelPasswordReset: () => {
      setResettingPassword(false);
      state.setPassword("");
    },
    submit: () => {
      if (!save.pending) {
        save.save(state.draft);
      }
    },
  };
}
