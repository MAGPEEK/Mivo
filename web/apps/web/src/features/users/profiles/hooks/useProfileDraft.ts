import type { AvatarColor } from "@mivo/api-client";
import { useState } from "react";

import { withPreset, type KnownAge, type KnownPreset } from "../../lib/contentLimit";
import { withLibrary } from "../../lib/libraryAccess";
import { pinDigits, type PinDraft } from "../lib/pin";
import type { ProfileDraft } from "../lib/profileDraft";

/** The draft of the profile editor and one setter per control. */
export interface ProfileDraftState {
  readonly draft: ProfileDraft;
  readonly setName: (name: string) => void;
  readonly setAvatarColor: (color: AvatarColor) => void;
  /** Takes what was typed into the PIN field; only digits are kept. */
  readonly typePin: (typed: string) => void;
  /** "Change PIN", "Remove PIN" and "Keep the current PIN". */
  readonly setPinAction: (action: PinDraft["action"]) => void;
  readonly setAllLibraries: (all: boolean) => void;
  readonly setLibrary: (libraryId: string, selected: boolean) => void;
  readonly setPreset: (preset: KnownPreset) => void;
  readonly setCustomAge: (age: KnownAge) => void;
  readonly setHideUnrated: (hide: boolean) => void;
}

function pinFor(action: PinDraft["action"]): PinDraft {
  return action === "set" ? { action, value: "" } : { action };
}

/** Holds the draft of the profile editor, starting from `initial`. */
export function useProfileDraft(initial: ProfileDraft): ProfileDraftState {
  const [draft, setDraft] = useState(initial);
  const change = (patch: (current: ProfileDraft) => Partial<ProfileDraft>) => {
    setDraft((current) => ({ ...current, ...patch(current) }));
  };
  return {
    draft,
    setName: (name) => {
      change(() => ({ name }));
    },
    setAvatarColor: (avatarColor) => {
      change(() => ({ avatarColor }));
    },
    typePin: (typed) => {
      change(() => ({ pin: { action: "set", value: pinDigits(typed) } }));
    },
    setPinAction: (action) => {
      change(() => ({ pin: pinFor(action) }));
    },
    setAllLibraries: (all) => {
      change((current) => ({ access: { ...current.access, all } }));
    },
    setLibrary: (libraryId, selected) => {
      change((current) => ({ access: withLibrary(current.access, libraryId, selected) }));
    },
    setPreset: (preset) => {
      change((current) => ({ limit: withPreset(current.limit, preset) }));
    },
    setCustomAge: (customAge) => {
      change((current) => ({ limit: { ...current.limit, customAge } }));
    },
    setHideUnrated: (hideUnrated) => {
      change((current) => ({ limit: { ...current.limit, hideUnrated } }));
    },
  };
}
