import { ApiError } from "@mivo/api-client";

import { isForbidden, isNotImplemented } from "../../lib/userProblems";
import type { ProfileField } from "./profileDraft";

/** Where a failed call is shown: next to a field, or above the dialog buttons. */
export type ProfileProblemPlace = ProfileField | "form";

/** Messages a failed profile call can show. */
export type ProfileProblemMessage =
  | "nameTaken"
  | "nameInvalid"
  | "pinInvalid"
  | "accessInvalid"
  | "limitInvalid"
  | "profileLimit"
  | "primaryProfile"
  | "forbidden"
  | "notImplemented"
  | "notFound"
  | "failed";

/** A failed call as the editor shows it. */
export interface ProfileProblem {
  readonly place: ProfileProblemPlace;
  readonly message: ProfileProblemMessage;
}

// The contract names these `field` values for "invalid".
const invalidFields: ReadonlyMap<string, ProfileProblem> = new Map<string, ProfileProblem>([
  ["name", { place: "name", message: "nameInvalid" }],
  ["pin", { place: "pin", message: "pinInvalid" }],
  ["libraryAccess", { place: "libraryAccess", message: "accessInvalid" }],
  ["contentLimit", { place: "contentLimit", message: "limitInvalid" }],
]);

// The problem codes of createProfile, updateProfile and deleteProfile.
const codeProblems: ReadonlyMap<string, ProfileProblem> = new Map<string, ProfileProblem>([
  ["name_taken", { place: "name", message: "nameTaken" }],
  ["profile_limit", { place: "form", message: "profileLimit" }],
  // Only deleting a primary profile answers "conflict"; the UI offers it for managed ones only.
  ["conflict", { place: "form", message: "primaryProfile" }],
  ["not_found", { place: "form", message: "notFound" }],
]);

const failed: ProfileProblem = { place: "form", message: "failed" };

/** Maps a failed create, update or delete of a profile to its message and place. */
export function profileProblem(error: unknown): ProfileProblem {
  if (isNotImplemented(error)) {
    return { place: "form", message: "notImplemented" };
  }
  if (isForbidden(error)) {
    return { place: "form", message: "forbidden" };
  }
  if (!(error instanceof ApiError)) {
    return failed;
  }
  if (error.code === "invalid" && error.field !== undefined) {
    return invalidFields.get(error.field) ?? failed;
  }
  return codeProblems.get(error.code) ?? failed;
}
