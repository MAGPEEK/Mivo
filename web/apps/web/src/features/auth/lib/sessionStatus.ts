import { ApiError, type Me, type SetupState } from "@mivo/api-client";

/**
 * Where the visitor stands (ADR 0032): `loading` until the server answered, `setupRequired`
 * while no account exists, `signedOut` without a session, `needsProfile` signed in without a
 * selected profile, `ready` otherwise.
 */
export type SessionStatus = "loading" | "setupRequired" | "signedOut" | "needsProfile" | "ready";

/** The session as the app sees it. */
export interface SessionState {
  readonly status: SessionStatus;
  /** The signed-in account; present for `needsProfile` and a signed-in `ready`. */
  readonly me: Me | undefined;
  /**
   * False when the server does not offer accounts yet (it answers `501 not_implemented`, before
   * M7-25) or could not tell: the app then runs without any guard, as before accounts.
   */
  readonly authAvailable: boolean;
}

/** The part of a TanStack Query result the derivation needs. */
export interface QuerySnapshot<Data> {
  readonly data: Data | undefined;
  readonly error: unknown;
  readonly isPending: boolean;
}

const unauthorizedStatus = 401;
const notImplementedStatus = 501;
const setupRequiredCode = "setup_required";

const loading: SessionState = { status: "loading", me: undefined, authAvailable: true };
const setupRequired: SessionState = { status: "setupRequired", me: undefined, authAvailable: true };
const signedOut: SessionState = { status: "signedOut", me: undefined, authAvailable: true };
const withoutAuth: SessionState = { status: "ready", me: undefined, authAvailable: false };

function hasStatus(error: unknown, status: number): error is ApiError {
  return error instanceof ApiError && error.status === status;
}

// The body is untrusted: an answer without an account object counts as "cannot tell".
function isMe(data: unknown): data is Me {
  if (typeof data !== "object" || data === null) {
    return false;
  }
  const { account, currentProfile } = data as Record<string, unknown>;
  return typeof account === "object" && account !== null && currentProfile !== undefined;
}

function fromMe(me: QuerySnapshot<Me>): SessionState {
  if (hasStatus(me.error, unauthorizedStatus)) {
    return me.error.code === setupRequiredCode ? setupRequired : signedOut;
  }
  if (!isMe(me.data)) {
    // 501 before accounts exist, a server or network failure, or a malformed body: the pages
    // show their own errors, and a later 401 still leads to sign-in.
    return withoutAuth;
  }
  const status = me.data.currentProfile === null ? "needsProfile" : "ready";
  return { status, me: me.data, authAvailable: true };
}

/** Derives the session from the answers of `GET /setup` and `GET /me`. */
export function deriveSession(
  setup: QuerySnapshot<SetupState>,
  me: QuerySnapshot<Me>,
): SessionState {
  if (setup.data?.required === true) {
    return setupRequired;
  }
  if (setup.isPending || me.isPending) {
    return loading;
  }
  if (hasStatus(setup.error, notImplementedStatus)) {
    return withoutAuth;
  }
  return fromMe(me);
}
