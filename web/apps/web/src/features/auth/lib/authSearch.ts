import { sanitizeNextPath } from "./nextPath";

/** Why the sign-in page opens: after "Sign out" (AU-05) or a lost session (AU-06). */
export type SignInReason = "signed-out" | "expired";

/** Search of the sign-in and profile pages. */
export interface AuthSearch {
  /** The same-app page to open afterwards; always sanitised. */
  readonly next?: string | undefined;
  /** Only on the sign-in page. */
  readonly reason?: SignInReason | undefined;
}

const signInReasons: readonly string[] = ["signed-out", "expired"] satisfies SignInReason[];

function isSignInReason(value: unknown): value is SignInReason {
  return typeof value === "string" && signInReasons.includes(value);
}

/** Validates the search of an auth page: unknown or unsafe values are dropped. */
export function validateAuthSearch(search: Record<string, unknown>): AuthSearch {
  return {
    next: sanitizeNextPath(search["next"]),
    reason: isSignInReason(search["reason"]) ? search["reason"] : undefined,
  };
}

/** Builds the href of an auth page with the given search; absent values are left out. */
export function authHref(pathname: string, search: AuthSearch = {}): string {
  const parameters = new URLSearchParams();
  if (search.next !== undefined) {
    parameters.set("next", search.next);
  }
  if (search.reason !== undefined) {
    parameters.set("reason", search.reason);
  }
  const query = parameters.toString();
  return query === "" ? pathname : `${pathname}?${query}`;
}
