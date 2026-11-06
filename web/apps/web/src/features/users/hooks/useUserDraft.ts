import type { AccountRole } from "@mivo/api-client";
import { useState } from "react";

import { withPreset, type KnownAge, type KnownPreset } from "../lib/contentLimit";
import { withLibrary } from "../lib/libraryAccess";
import type { UserDraft } from "../lib/userDraft";

/** The draft of the user dialog and one setter per control. */
export interface UserDraftState {
  readonly draft: UserDraft;
  readonly setName: (name: string) => void;
  readonly setPassword: (password: string) => void;
  readonly setRole: (role: AccountRole) => void;
  readonly setDisabled: (disabled: boolean) => void;
  readonly setAllLibraries: (all: boolean) => void;
  readonly setLibrary: (libraryId: string, selected: boolean) => void;
  readonly setPreset: (preset: KnownPreset) => void;
  readonly setCustomAge: (age: KnownAge) => void;
  readonly setHideUnrated: (hide: boolean) => void;
}

/** Holds the draft of the user dialog, starting from `initial`. */
export function useUserDraft(initial: UserDraft): UserDraftState {
  const [draft, setDraft] = useState(initial);
  const update = (change: (current: UserDraft) => UserDraft) => {
    setDraft(change);
  };
  return {
    draft,
    setName: (name) => {
      update((current) => ({ ...current, name }));
    },
    setPassword: (password) => {
      update((current) => ({ ...current, password }));
    },
    setRole: (role) => {
      update((current) => ({ ...current, role }));
    },
    setDisabled: (disabled) => {
      update((current) => ({ ...current, disabled }));
    },
    setAllLibraries: (all) => {
      update((current) => ({ ...current, access: { ...current.access, all } }));
    },
    setLibrary: (libraryId, selected) => {
      update((current) => ({
        ...current,
        access: withLibrary(current.access, libraryId, selected),
      }));
    },
    setPreset: (preset) => {
      update((current) => ({ ...current, limit: withPreset(current.limit, preset) }));
    },
    setCustomAge: (customAge) => {
      update((current) => ({ ...current, limit: { ...current.limit, customAge } }));
    },
    setHideUnrated: (hideUnrated) => {
      update((current) => ({ ...current, limit: { ...current.limit, hideUnrated } }));
    },
  };
}
