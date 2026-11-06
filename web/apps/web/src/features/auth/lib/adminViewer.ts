import type { SessionState } from "./sessionStatus";

/**
 * Whether the viewer may see the administration pages: the selected profile carries admin
 * powers (`me.currentProfile.isAdmin`). A server without accounts (`authAvailable` false)
 * runs without any guard, as before accounts, so its viewer counts as admin; the pages then
 * show the server's own "not available yet" answer.
 */
export function isAdminViewer(session: SessionState): boolean {
  if (!session.authAvailable) {
    return session.status === "ready";
  }
  return session.status === "ready" && session.me?.currentProfile?.isAdmin === true;
}
