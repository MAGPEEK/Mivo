import { useState } from "react";

/**
 * Whether the folder heading takes focus when it appears: false for the folder the page was
 * opened on (a reload or a link must not grab focus), true once the user moved to another
 * folder, so keyboard and screen reader users land on the new folder (the shell has no route
 * announcer and sets no document title).
 */
export function useFolderHeadingFocus(path: string): boolean {
  const [state, setState] = useState({ openedWith: path, moved: false });
  const moved = state.moved || path !== state.openedWith;
  if (moved && !state.moved) {
    // Adjusting state while rendering is the supported way to derive it from a prop; it keeps
    // the answer true after the user navigates back to the folder the page opened with.
    setState({ ...state, moved: true });
  }
  return moved;
}
