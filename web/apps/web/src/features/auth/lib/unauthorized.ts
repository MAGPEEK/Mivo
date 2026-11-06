// What the app does with a cross-cutting answer of any API request (ADR 0032): a 401 means the
// session is gone, a 403 "profile_required" that the session has no profile selected.

/** Where an answer sends the visitor; `undefined` leaves it to the caller of the request. */
export type UnauthorizedAction = "signIn" | "setup" | "profiles";

/** The facts of one answer the decision needs. */
export interface AnswerFacts {
  readonly method: string;
  readonly schemaPath: string;
  readonly status: number;
  /** The problem `code`, when the body had one. */
  readonly code: string | undefined;
}

const unauthorizedStatus = 401;
const forbiddenStatus = 403;
const setupRequiredCode = "setup_required";
const profileRequiredCode = "profile_required";

// Their 401 is an answer for the form or the session query, not a lost session: a wrong
// password at sign-in, a sign-out that finds no session, the session check itself.
const ownUnauthorizedAnswers: readonly string[] = [
  "POST /api/v1/auth/session",
  "DELETE /api/v1/auth/session",
  "GET /api/v1/setup",
  "POST /api/v1/setup",
  "GET /api/v1/me",
];

/** Decides what an answer means for the session; most answers mean nothing. */
export function unauthorizedAction(facts: AnswerFacts): UnauthorizedAction | undefined {
  if (facts.status === forbiddenStatus) {
    return facts.code === profileRequiredCode ? "profiles" : undefined;
  }
  if (facts.status !== unauthorizedStatus) {
    return undefined;
  }
  if (ownUnauthorizedAnswers.includes(`${facts.method} ${facts.schemaPath}`)) {
    return undefined;
  }
  return facts.code === setupRequiredCode ? "setup" : "signIn";
}

/** Whether answers with `status` may need {@link unauthorizedAction}; spares reading others. */
export function mayAffectSession(status: number): boolean {
  return status === unauthorizedStatus || status === forbiddenStatus;
}
