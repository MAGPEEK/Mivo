// Paths of the auth pages. The router declares the routes with the same strings; the guard and
// the 401 handling use them to decide where a visitor belongs.

/** Sign-in page (AU-01, AU-05, AU-06). */
export const signInPath = "/sign-in";

/** First-run setup step 1 "Create admin account" (AU-02). */
export const setupPath = "/setup";

/** "Setup only from the local network" (AU-03). */
export const remoteSetupPath = "/setup/remote";

/** Profile picker (US-01, owned by M7-31; a placeholder until then). */
export const profilesPath = "/profiles";

/** Home, where a visitor lands when there is no `next` page to return to. */
export const homePath = "/";

/** Pages a visitor without a session may open; everything else needs one. */
export const publicAuthPaths: readonly string[] = [signInPath, setupPath, remoteSetupPath];

/** Pages that belong to the sign-in flow and are never a target to return to. */
export const authFlowPaths: readonly string[] = [...publicAuthPaths, profilesPath];

/** Whether `pathname` is one of the sign-in flow pages. */
export function isAuthFlowPath(pathname: string): boolean {
  return authFlowPaths.includes(pathname);
}
