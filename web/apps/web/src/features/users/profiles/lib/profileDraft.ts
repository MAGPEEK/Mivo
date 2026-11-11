import type {
  AvatarColor,
  LibraryAccess,
  Profile,
  ProfileCreate,
  ProfileUpdate,
} from "@mivo/api-client";

import {
  contentLimitDraft,
  noContentLimit,
  sameContentLimit,
  toContentLimit,
  type ContentLimitDraft,
} from "../../lib/contentLimit";
import {
  allLibraries,
  libraryAccessDraft,
  sameLibraryAccess,
  type LibraryAccessDraft,
} from "../../lib/libraryAccess";
import { nameProblem, type DraftProblems } from "../../lib/userDraft";
import { initialPin, pinChange, pinProblem, type PinDraft, type PinProblem } from "./pin";
import { toProfileAccess, type LibraryChoice } from "./profileAccess";

/** Managed profiles an account may have at most (api/openapi.yaml, "profile_limit"). */
export const maxManagedProfiles = 10;

/** The account a profile belongs to, as far as the editor needs it. */
export interface ProfileOwner {
  readonly id: string;
  readonly name: string;
  /** The account's own libraries: the most a managed profile can get. */
  readonly libraryAccess: LibraryAccess;
}

/** A profile as the editor edits it. */
export interface ProfileDraft {
  readonly name: string;
  readonly avatarColor: AvatarColor;
  readonly pin: PinDraft;
  readonly limit: ContentLimitDraft;
  readonly access: LibraryAccessDraft;
}

/** The draft of a new managed profile: the contract's defaults, no PIN. */
export const newProfileDraft: ProfileDraft = {
  name: "",
  avatarColor: "coral",
  pin: initialPin(false),
  limit: noContentLimit,
  access: allLibraries,
};

/** The draft of an existing profile. */
export function profileDraft(profile: Profile): ProfileDraft {
  return {
    name: profile.name,
    avatarColor: profile.avatarColor,
    pin: initialPin(profile.hasPin),
    limit: contentLimitDraft(profile.contentLimit),
    access: libraryAccessDraft(profile.libraryAccess),
  };
}

/** Fields a local check or the server can flag. */
export type ProfileField = "name" | "pin" | "libraryAccess" | "contentLimit";

/** Local problems of a draft, by field; empty when the draft may be saved. */
export interface ProfileDraftProblems {
  name?: NonNullable<DraftProblems["name"]>;
  pin?: PinProblem;
}

/**
 * Checks what the server would refuse anyway. `original` is the profile being edited: the
 * name of a primary profile belongs to its account and is not checked here.
 */
export function profileDraftProblems(
  draft: ProfileDraft,
  original: Profile | undefined,
): ProfileDraftProblems {
  const problems: ProfileDraftProblems = {};
  const name = original?.isPrimary === true ? undefined : nameProblem(draft.name);
  if (name !== undefined) {
    problems.name = name;
  }
  const pin = pinProblem(draft.pin, original?.hasPin === true);
  if (pin !== undefined) {
    problems.pin = pin;
  }
  return problems;
}

/** The body of `createProfile`. `choices` are the libraries that exist now. */
export function toProfileCreateBody(
  draft: ProfileDraft,
  choices: readonly LibraryChoice[],
): ProfileCreate {
  const pin = pinChange(draft.pin);
  return {
    name: draft.name.trim(),
    avatarColor: draft.avatarColor,
    contentLimit: toContentLimit(draft.limit),
    libraryAccess: toProfileAccess(draft.access, choices),
    ...(typeof pin === "string" ? { pin } : {}),
  };
}

/**
 * The body of `updateProfile` with only what changed, so untouched properties stay as they
 * are. A primary profile takes name, libraries and limit from its account; the contract
 * allows only its colour and PIN here.
 */
export function toProfileUpdateBody(
  draft: ProfileDraft,
  original: Profile,
  choices: readonly LibraryChoice[],
): ProfileUpdate {
  const before = profileDraft(original);
  const body: ProfileUpdate = {};
  if (draft.avatarColor !== before.avatarColor) {
    body.avatarColor = draft.avatarColor;
  }
  const pin = pinChange(draft.pin);
  if (pin !== undefined) {
    body.pin = pin;
  }
  if (original.isPrimary) {
    return body;
  }
  const name = draft.name.trim();
  if (name !== before.name) {
    body.name = name;
  }
  if (!sameContentLimit(draft.limit, before.limit)) {
    body.contentLimit = toContentLimit(draft.limit);
  }
  if (!sameLibraryAccess(draft.access, before.access)) {
    body.libraryAccess = toProfileAccess(draft.access, choices);
  }
  return body;
}
