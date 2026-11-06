import { isAdminViewer } from "../lib/adminViewer";
import { useSession } from "./useSession";

/** Whether the viewer may see the administration pages; see {@link isAdminViewer}. */
export function useAdminViewer(): boolean {
  return isAdminViewer(useSession());
}
