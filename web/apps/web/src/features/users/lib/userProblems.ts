import { ApiError } from "@mivo/api-client";

import type { UserField } from "./userDraft";

const forbiddenStatus = 403;
const notImplementedStatus = 501;

/** Where a failed call is shown: next to a field, or above the dialog buttons. */
export type ProblemPlace = UserField | "form";

/** Messages a failed user administration call can show. */
export type ProblemMessage =
  | "nameTaken"
  | "nameInvalid"
  | "passwordInvalid"
  | "accessInvalid"
  | "limitInvalid"
  | "lastAdmin"
  | "forbidden"
  | "notImplemented"
  | "notFound"
  | "failed";

/** A failed call as the dialog shows it. */
export interface UserProblem {
  readonly place: ProblemPlace;
  readonly message: ProblemMessage;
}

// The contract names these `field` values for "invalid".
const invalidFields: Readonly<Record<string, UserProblem>> = {
  name: { place: "name", message: "nameInvalid" },
  password: { place: "password", message: "passwordInvalid" },
  libraryAccess: { place: "libraryAccess", message: "accessInvalid" },
  contentLimit: { place: "contentLimit", message: "limitInvalid" },
};

/** Whether the server does not serve user administration yet (`501 not_implemented`). */
export function isNotImplemented(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    (error.status === notImplementedStatus || error.code === "not_implemented")
  );
}

/** Whether the server refused the viewer (not an admin, or not on the local network). */
export function isForbidden(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === forbiddenStatus || error.code === "forbidden")
  );
}

function codeProblem(code: string): UserProblem | undefined {
  switch (code) {
    case "name_taken":
      return { place: "name", message: "nameTaken" };
    case "last_admin":
      return { place: "form", message: "lastAdmin" };
    case "not_found":
      return { place: "form", message: "notFound" };
    default:
      return undefined;
  }
}

/** Maps a failed create, update, reset or delete call to its message and place. */
export function userProblem(error: unknown): UserProblem {
  if (isNotImplemented(error)) {
    return { place: "form", message: "notImplemented" };
  }
  if (isForbidden(error)) {
    return { place: "form", message: "forbidden" };
  }
  if (!(error instanceof ApiError)) {
    return { place: "form", message: "failed" };
  }
  if (error.code === "invalid" && error.field !== undefined) {
    const fieldProblem = invalidFields[error.field];
    if (fieldProblem !== undefined) {
      return fieldProblem;
    }
  }
  return codeProblem(error.code) ?? { place: "form", message: "failed" };
}
