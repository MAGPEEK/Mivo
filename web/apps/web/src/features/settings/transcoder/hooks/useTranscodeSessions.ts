import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApiClient } from "../../../../shared/api";
import {
  isForbidden,
  stopSession,
  transcoderSessionsQueryKey,
  transcoderSessionsQueryOptions,
  type SessionSummary,
} from "../api/transcoderApi";

/** Why the last stop request failed; null while nothing failed. */
export type StopFailure = "forbidden" | "failed" | null;

/** State and actions of the "Stop session" confirmation. */
export interface SessionStop {
  /** The session the dialog asks about; null while the dialog is closed. */
  readonly target: SessionSummary | null;
  readonly pending: boolean;
  readonly failure: StopFailure;
  readonly ask: (session: SessionSummary) => void;
  readonly cancel: () => void;
  readonly confirm: () => void;
}

/** What the sessions table shows. */
export type SessionsLoad =
  | { readonly state: "loading" }
  | { readonly state: "forbidden" }
  | { readonly state: "error"; readonly retry: () => void }
  | {
      readonly state: "ready";
      readonly sessions: readonly SessionSummary[];
      readonly stop: SessionStop;
    };

function useSessionStop(): SessionStop {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<SessionSummary | null>(null);
  const mutation = useMutation({
    mutationFn: (sessionId: string) => stopSession(client, sessionId),
    onSuccess: () => {
      setTarget(null);
    },
    // Refresh at once instead of waiting for the next poll, whatever the outcome was.
    onSettled: () => queryClient.invalidateQueries({ queryKey: transcoderSessionsQueryKey }),
  });
  const { reset, mutate } = mutation;
  let failure: StopFailure = null;
  if (mutation.isError) {
    failure = isForbidden(mutation.error) ? "forbidden" : "failed";
  }
  return {
    target,
    pending: mutation.isPending,
    failure,
    ask: (session) => {
      reset();
      setTarget(session);
    },
    cancel: () => {
      // Closing mid-request would hide the outcome of a stop that still runs.
      if (!mutation.isPending) {
        reset();
        setTarget(null);
      }
    },
    confirm: () => {
      if (target !== null) {
        mutate(target.id);
      }
    },
  };
}

/** The running playback sessions, polled while the page is visible, and the stop action. */
export function useTranscodeSessions(): SessionsLoad {
  const client = useApiClient();
  const query = useQuery(transcoderSessionsQueryOptions(client));
  const stop = useSessionStop();
  if (query.data !== undefined) {
    return { state: "ready", sessions: query.data.sessions, stop };
  }
  if (query.error === null) {
    return { state: "loading" };
  }
  return isForbidden(query.error)
    ? { state: "forbidden" }
    : { state: "error", retry: () => void query.refetch() };
}
