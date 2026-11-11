import type { Profile } from "@mivo/api-client";

import { useLibraryList, type LibraryLoadStatus } from "../../hooks/useLibraryList";
import { libraryChoices, type LibraryChoice } from "../lib/profileAccess";
import { newProfileDraft, profileDraft, type ProfileOwner } from "../lib/profileDraft";
import { useProfileDraft, type ProfileDraftState } from "./useProfileDraft";
import { useProfileSave, type ProfileErrors } from "./useProfileSave";

/** What the profile editor edits. */
export interface ProfileDialogTarget {
  readonly owner: ProfileOwner;
  /** The profile to edit; omitted for "Create managed profile". */
  readonly profile?: Profile | undefined;
}

/** Everything the profile editor renders and does. */
export interface ProfileDialogState extends ProfileDraftState {
  /** True for "Create managed profile", false when editing a profile. */
  readonly creating: boolean;
  /** True for a primary profile: only its colour and PIN are edited here. */
  readonly primary: boolean;
  /** True when the edited profile has a stored PIN. */
  readonly hasPin: boolean;
  /** Every library, marked by whether the account may see it. */
  readonly choices: readonly LibraryChoice[];
  /** The editor saves only once the libraries are `ready`. */
  readonly librariesStatus: LibraryLoadStatus;
  readonly retryLibraries: () => void;
  readonly errors: ProfileErrors;
  readonly pending: boolean;
  /** False while a save runs or the libraries are not loaded. */
  readonly canSave: boolean;
  /** Checks and saves the draft; the one Save button of the editor. */
  readonly submit: () => void;
}

/** State of the editor that adds a managed profile to `owner`, or edits `profile`. */
export function useProfileDialog(
  { owner, profile }: ProfileDialogTarget,
  onSaved: (name: string) => void,
): ProfileDialogState {
  const state = useProfileDraft(profile === undefined ? newProfileDraft : profileDraft(profile));
  const libraryList = useLibraryList();
  const choices = libraryChoices(libraryList.libraries, owner.libraryAccess);
  const primary = profile?.isPrimary === true;
  const save = useProfileSave({ ownerId: owner.id, profile }, () => {
    onSaved(primary ? owner.name : state.draft.name.trim());
  });
  // A primary profile sends no libraries, so it does not have to wait for them.
  const canSave = (primary || libraryList.status === "ready") && !save.pending;
  return {
    ...state,
    creating: profile === undefined,
    primary,
    hasPin: profile?.hasPin === true,
    choices,
    librariesStatus: libraryList.status,
    retryLibraries: libraryList.retry,
    errors: save.errors,
    pending: save.pending,
    canSave,
    submit: () => {
      if (canSave) {
        save.save(state.draft, choices);
      }
    },
  };
}
