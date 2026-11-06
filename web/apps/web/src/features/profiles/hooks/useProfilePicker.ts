import type { Me, ProfileSummary } from "@mivo/api-client";
import { useState } from "react";

import { needsPin, primaryProfile, rememberByDefault } from "../lib/profileModel";
import type { SelectionFailure } from "../lib/selectionFailure";
import { usePinEntry, type PinEntry } from "./usePinEntry";
import { useSelectProfile } from "./useSelectProfile";

/** The profile waiting for its PIN, with the page to open afterwards. */
interface PinRequest {
  readonly profile: ProfileSummary;
  readonly next: string | undefined;
}

/** State and actions of the profile picker (US-01…03). */
export interface ProfilePicker {
  /** The profile whose PIN pad is open. */
  readonly pinProfile: ProfileSummary | undefined;
  readonly pin: PinEntry;
  /** "Remember on this device"; reset to the profile's default whenever a pad opens. */
  readonly remember: boolean;
  readonly setRemember: (remember: boolean) => void;
  /** A failure of choosing a profile without PIN. */
  readonly failure: SelectionFailure | undefined;
  readonly pending: boolean;
  readonly choose: (profile: ProfileSummary) => void;
  /** "Add profile": the primary profile first (with its PIN), then the editor. */
  readonly addProfile: (editorPath: string) => void;
  readonly cancelPin: () => void;
}

/** Drives the picker: direct selection, the PIN pad and "Add profile". */
export function useProfilePicker(me: Me): ProfilePicker {
  const [pinRequest, setPinRequest] = useState<PinRequest>();
  const [remember, setRemember] = useState(false);
  const [failure, setFailure] = useState<SelectionFailure>();
  const selection = useSelectProfile();
  const pin = usePinEntry({
    busy: selection.pending,
    onComplete: (entered, reject) => {
      if (pinRequest !== undefined) {
        const choice = { profileId: pinRequest.profile.id, pin: entered, remember };
        selection.select({ choice, next: pinRequest.next }, reject);
      }
    },
  });
  const start = (profile: ProfileSummary, next: string | undefined): void => {
    setFailure(undefined);
    pin.reset();
    if (needsPin(profile)) {
      setRemember(rememberByDefault(profile));
      setPinRequest({ profile, next });
      return;
    }
    setPinRequest(undefined);
    selection.select({ choice: { profileId: profile.id }, next }, setFailure);
  };
  return {
    pinProfile: pinRequest?.profile,
    pin,
    remember,
    setRemember,
    failure,
    pending: selection.pending,
    choose: (profile) => {
      start(profile, undefined);
    },
    addProfile: (editorPath) => {
      const primary = primaryProfile(me);
      if (primary !== undefined) {
        start(primary, editorPath);
      }
    },
    cancelPin: () => {
      setPinRequest(undefined);
      pin.reset();
    },
  };
}
