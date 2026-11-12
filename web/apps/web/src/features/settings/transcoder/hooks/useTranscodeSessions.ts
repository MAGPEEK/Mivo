import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useApiClient } from "../../../../shared/api";
import {
  isForbidden,
  stopSession,
  transcoderSessionsQueryKey,
  transcoderSessionsQueryOptions,
  type SessionSummary,
} from "../api/transcoderApi";

/** What the sessions table shows. */
export type SessionsLoad =
  | { readonly state: "loading" }
  | { readonly state: "forbidden" }
  | { readonly state: "error"; readonly retry: () => void }
  | {
      readonly state: "ready";
      readonly sessions: readonly SessionSummary[];
      /** ID of the session whose stop request is in flight; null otherwise. */
      readonly stoppingId: string | null;
      /** Whether the last stop request failed. */
      readonly stopFailed: boolean;
      readonly stop: (sessionId: string) => void;
    };

/**
 * The running playback sessions, polled while the page is visible, and the stop action. A
 * stop refreshes the list at once instead of waiting for the next poll.
 */
export function useTranscodeSessions(): SessionsLoad {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const query = useQuery(transcoderSessionsQueryOptions(client));
  const stopping = useMutation({
    mutationFn: (sessionId: string) => stopSession(client, sessionId),
    onSettled: () => queryClient.invalidateQueries({ queryKey: transcoderSessionsQueryKey }),
  });
  if (query.data !== undefined) {
    return {
      state: "ready",
      sessions: query.data.sessions,
      stoppingId: stopping.isPending ? stopping.variables : null,
      stopFailed: stopping.isError,
      stop: (sessionId) => {
        stopping.mutate(sessionId);
      },
    };
  }
  if (query.error === null) {
    return { state: "loading" };
  }
  return isForbidden(query.error)
    ? { state: "forbidden" }
    : { state: "error", retry: () => void query.refetch() };
}
