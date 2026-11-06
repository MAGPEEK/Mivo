import type { AccountRole, User, UserCreate, UserUpdate } from "@mivo/api-client";

import {
  contentLimitDraft,
  noContentLimit,
  sameContentLimit,
  toContentLimit,
  type ContentLimitDraft,
} from "./contentLimit";
import {
  allLibraries,
  libraryAccessDraft,
  sameLibraryAccess,
  toLibraryAccess,
  type LibraryAccessDraft,
} from "./libraryAccess";

/** An account as the create/edit dialog edits it. */
export interface UserDraft {
  readonly name: string;
  /** The initial password (create) or the new one after "Reset password" (edit); "" for none. */
  readonly password: string;
  readonly role: AccountRole;
  readonly disabled: boolean;
  readonly access: LibraryAccessDraft;
  readonly limit: ContentLimitDraft;
}

/** Length limit of an account name in the API (characters after NFC normalisation). */
export const maxNameLength = 40;
/** Password length limits in the API, in bytes of UTF-8. */
export const minPasswordBytes = 8;
export const maxPasswordBytes = 128;

/** The draft of a new account: role user, all libraries, no limit. */
export const newUserDraft: UserDraft = {
  name: "",
  password: "",
  role: "user",
  disabled: false,
  access: allLibraries,
  limit: noContentLimit,
};

/** The draft of an existing account. */
export function userDraft(user: User): UserDraft {
  return {
    name: user.name,
    password: "",
    role: user.role,
    disabled: user.status === "disabled",
    access: libraryAccessDraft(user.libraryAccess),
    limit: contentLimitDraft(user.contentLimit),
  };
}

/** Fields a local check or the server can flag. */
export type UserField = "name" | "password" | "libraryAccess" | "contentLimit";

/** Local problems of a draft, by field; empty when the draft may be saved. */
export interface DraftProblems {
  name?: "required" | "tooLong";
  password?: "required" | "tooShort" | "tooLong";
}

function passwordProblem(password: string, required: boolean): DraftProblems["password"] {
  if (password === "") {
    return required ? "required" : undefined;
  }
  const bytes = new TextEncoder().encode(password).length;
  if (bytes < minPasswordBytes) {
    return "tooShort";
  }
  return bytes > maxPasswordBytes ? "tooLong" : undefined;
}

/**
 * Checks what the server would refuse anyway, so the common mistakes get an answer without a
 * round trip. `creating` makes the initial password required.
 */
export function draftProblems(draft: UserDraft, creating: boolean): DraftProblems {
  const name = draft.name.trim().normalize("NFC");
  const problems: DraftProblems = {};
  if (name === "") {
    problems.name = "required";
  } else if (Array.from(name).length > maxNameLength) {
    problems.name = "tooLong";
  }
  const password = passwordProblem(draft.password, creating);
  if (password !== undefined) {
    problems.password = password;
  }
  return problems;
}

// An admin account always has every library, so the contract forbids sending a list for it.
function accessFor(draft: UserDraft, libraryIds: readonly string[]) {
  return draft.role === "admin" ? undefined : toLibraryAccess(draft.access, libraryIds);
}

/** The body of `createUser`. `libraryIds` are the libraries that exist now. */
export function toCreateBody(draft: UserDraft, libraryIds: readonly string[]): UserCreate {
  const libraryAccess = accessFor(draft, libraryIds);
  return {
    name: draft.name.trim(),
    password: draft.password,
    role: draft.role,
    contentLimit: toContentLimit(draft.limit),
    ...(libraryAccess === undefined ? {} : { libraryAccess }),
  };
}

/**
 * The body of `updateUser` with only what changed: properties the dialog did not touch stay
 * as they are, including values of a newer server this client does not know. A demotion to
 * user always carries the access, as the contract requires.
 */
export function toUpdateBody(
  draft: UserDraft,
  original: UserDraft,
  libraryIds: readonly string[],
): UserUpdate {
  const body: UserUpdate = {};
  const name = draft.name.trim();
  if (name !== original.name) {
    body.name = name;
  }
  if (draft.role !== original.role) {
    body.role = draft.role;
  }
  if (draft.disabled !== original.disabled) {
    body.status = draft.disabled ? "disabled" : "active";
  }
  if (!sameContentLimit(draft.limit, original.limit)) {
    body.contentLimit = toContentLimit(draft.limit);
  }
  const accessChanged = !sameLibraryAccess(draft.access, original.access);
  const libraryAccess = accessFor(draft, libraryIds);
  if (libraryAccess !== undefined && (accessChanged || body.role !== undefined)) {
    body.libraryAccess = libraryAccess;
  }
  return body;
}
