import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { createPlaybackApi } from "../api/playbackApi";

/** The speed changes about once per second on the server; every five seconds is enough to read. */
export const liveSessionIntervalMilliseconds = 5000;

/**
 * The transcode speed of the running session, read from `GET …/sessions/{id}` every five
 * seconds but only while `active` (the stream-info overlay is open), so a closed overlay costs
 * the server nothing. `undefined` until the first answer, when the session has no speed (direct
 * play) or the server cannot answer.
 */
export function useLiveTranscodeSpeed(
  sessionId: string | undefined,
  active: boolean,
): number | undefined {
  const client = useApiClient();
  const query = useQuery({
    queryKey: ["player", "liveSession", sessionId],
    queryFn: () => createPlaybackApi(client).getSession(sessionId ?? ""),
    enabled: active && sessionId !== undefined,
    refetchInterval: liveSessionIntervalMilliseconds,
    retry: false,
    // A reopened overlay must not show the speed of the last opening.
    gcTime: 0,
  });
  return active ? query.data?.transcodeSpeed : undefined;
}
