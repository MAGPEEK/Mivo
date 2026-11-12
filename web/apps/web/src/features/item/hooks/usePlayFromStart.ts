import { useNavigate } from "@tanstack/react-router";

import type { PlayChoice } from "../lib/playChoice";
import { playChoiceState } from "../lib/playChoiceStorage";

/** What "Play from start" opens: the item and, for a movie or episode, its file. */
export interface PlayFromStartTarget {
  readonly itemId: string;
  readonly fileId?: string;
  /** The tracks the viewer picked, handed to the player like Play does. */
  readonly choice?: PlayChoice;
}

/**
 * Returns the action behind "Play from start": opens the player without a start position, so
 * playback begins at 0 whatever the stored resume position is.
 */
export function usePlayFromStart({ itemId, fileId, choice }: PlayFromStartTarget): () => void {
  const navigate = useNavigate();
  const state = choice === undefined ? undefined : playChoiceState(choice);
  return () => {
    void navigate({
      to: "/items/$itemId/play",
      params: { itemId },
      search: { fileId },
      ...(state === undefined ? {} : { state }),
    });
  };
}
