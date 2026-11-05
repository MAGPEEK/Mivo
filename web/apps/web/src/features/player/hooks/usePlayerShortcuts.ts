import { seekStepSeconds } from "../lib/playerKeys";
import { usePlayerKeyboard } from "./usePlayerKeyboard";

/** The page's actions the keyboard map triggers. */
export interface ShortcutActions {
  readonly togglePlay: () => void;
  readonly seek: (position: number) => void;
  readonly toggleFullscreen: () => void;
  readonly toggleStreamInfo: () => void;
  readonly toggleMute: () => void;
}

/** The keyboard map (`playerKeys.ts`) on top of the page's actions. */
export function usePlayerShortcuts(
  actions: ShortcutActions,
  position: number,
  escape: () => void,
): void {
  usePlayerKeyboard({
    togglePlay: actions.togglePlay,
    seekBackward: () => {
      actions.seek(Math.max(0, position - seekStepSeconds));
    },
    seekForward: () => {
      actions.seek(position + seekStepSeconds);
    },
    toggleFullscreen: actions.toggleFullscreen,
    toggleStreamInfo: actions.toggleStreamInfo,
    toggleMute: actions.toggleMute,
    escape,
  });
}
