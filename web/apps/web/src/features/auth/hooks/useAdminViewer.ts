import { isAdminViewer } from "../lib/adminViewer";
import { useSessionSnapshot } from "./useSession";

/**
 * Whether the viewer may see the administration pages; see {@link isAdminViewer}. It reads the
 * session as the gate last saw it: every caller sits below the gate, and a second check on
 * mount would only repeat the gate's request.
 */
export function useAdminViewer(): boolean {
  return isAdminViewer(useSessionSnapshot());
}
