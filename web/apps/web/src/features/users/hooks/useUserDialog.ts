import type { Library, User } from "@mivo/api-client";
import { useState } from "react";

import { newUserDraft, userDraft } from "../lib/userDraft";
import { useLibraryList, type LibraryLoadStatus } from "./useLibraryList";
import { useUserDraft, type UserDraftState } from "./useUserDraft";
import { useUserSave, type UserErrors } from "./useUserSave";

/** Everything the create/edit dialog renders and does. */
export interface UserDialogState extends UserDraftState {
  /** True for "Add user", false when editing an account. */
  readonly creating: boolean;
  readonly libraries: readonly Library[];
  /** The dialog saves only once the libraries are `ready`. */
  readonly librariesStatus: LibraryLoadStatus;
  readonly retryLibraries: () => void;
  /** False while a save runs or the libraries are not loaded. */
  readonly canSave: boolean;
  readonly errors: UserErrors;
  readonly pending: boolean;
  /** True after "Reset password": the dialog shows the new-password field. */
  readonly resettingPassword: boolean;
  readonly startPasswordReset: () => void;
  readonly cancelPasswordReset: () => void;
  /** Checks and saves the draft; the one Save button of the dialog. */
  readonly submit: () => void;
}

/** State of the dialog that adds an account, or edits `user` when given. */
export function useUserDialog(
  user: User | undefined,
  onSaved: (name: string) => void,
): UserDialogState {
  const state = useUserDraft(user === undefined ? newUserDraft : userDraft(user));
  // Never save against libraries that are still loading or failed to load: the access would
  // go out as an empty list and the user would lose every library.
  const libraryList = useLibraryList();
  const { libraries } = libraryList;
  const [resettingPassword, setResettingPassword] = useState(false);
  const save = useUserSave(user, libraries, () => {
    onSaved(state.draft.name.trim());
  });
  const canSave = libraryList.status === "ready" && !save.pending;
  return {
    ...state,
    creating: user === undefined,
    libraries,
    librariesStatus: libraryList.status,
    retryLibraries: libraryList.retry,
    canSave,
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
      if (canSave) {
        save.save(state.draft);
      }
    },
  };
}
