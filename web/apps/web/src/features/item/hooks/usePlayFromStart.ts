import { useNavigate } from "@tanstack/react-router";

/** What "Play from start" opens: the item and, for a movie or episode, its file. */
export interface PlayFromStartTarget {
  readonly itemId: string;
  readonly fileId?: string;
}

/**
 * Returns the action behind "Play from start": opens the player without a start position, so
 * playback begins at 0 whatever the stored resume position is.
 */
export function usePlayFromStart({ itemId, fileId }: PlayFromStartTarget): () => void {
  const navigate = useNavigate();
  return () => {
    void navigate({ to: "/items/$itemId/play", params: { itemId }, search: { fileId } });
  };
}
