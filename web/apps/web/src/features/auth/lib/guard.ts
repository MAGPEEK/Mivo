import {
  homePath,
  isAuthFlowPath,
  profilesPath,
  remoteSetupPath,
  setupPath,
  signInPath,
} from "./authPaths";
import { authHref } from "./authSearch";
import { nextPathFor, sanitizeNextPath } from "./nextPath";
import type { SessionState } from "./sessionStatus";

/** The page the guard looks at. */
export interface GuardLocation {
  readonly pathname: string;
  /** Path, search and hash, e.g. `/items/1?fileId=2`. */
  readonly href: string;
  /** The raw `next` search value of the page, when it has one. */
  readonly next: unknown;
}

/** The page to return to: a flow page passes its own `next` on, any other page names itself. */
export function returnTarget(location: GuardLocation): string | undefined {
  return isAuthFlowPath(location.pathname)
    ? sanitizeNextPath(location.next)
    : nextPathFor(location.href);
}

const setupPages: readonly string[] = [setupPath, remoteSetupPath];

/**
 * The href the visitor must be sent to, or `undefined` when the page may be shown: setup while
 * no account exists, sign-in without a session, the profile picker without a profile, and away
 * from the flow pages once nothing is missing. `loading` never redirects.
 */
export function guardRedirect(session: SessionState, location: GuardLocation): string | undefined {
  const { pathname } = location;
  switch (session.status) {
    case "loading":
      return undefined;
    case "setupRequired":
      return setupPages.includes(pathname) ? undefined : setupPath;
    case "signedOut":
      return pathname === signInPath
        ? undefined
        : authHref(signInPath, { next: returnTarget(location) });
    case "needsProfile":
      return pathname === profilesPath
        ? undefined
        : authHref(profilesPath, { next: returnTarget(location) });
    case "ready":
      return isAuthFlowPath(pathname) ? (sanitizeNextPath(location.next) ?? homePath) : undefined;
  }
}
