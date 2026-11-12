import { useEffect, useState } from "react";

import { sessionToasts, type SessionToastKind } from "../lib/sessionToasts";
import type { PlaybackSnapshot } from "../lib/playbackEngine";

/** How long a toast stays when nobody closes it, in milliseconds. */
export const toastDurationMilliseconds = 8000;

/** The toasts shown and how to close them. */
export interface SessionToastsView {
  readonly toasts: readonly SessionToastKind[];
  readonly dismiss: () => void;
}

/**
 * Tells the viewer what a switch cost (PL-10, PL-12): the session a switch created reports a
 * refused burn-in or a quality cap, and the toasts for it show once, for that session only, and
 * fade after {@link toastDurationMilliseconds} or when closed.
 */
export function useSessionToasts(
  snapshot: Pick<PlaybackSnapshot, "session" | "switchedSessionId">,
): SessionToastsView {
  const { session, switchedSessionId } = snapshot;
  const [dismissedId, setDismissedId] = useState<string>();
  const announced = session !== undefined && session.id === switchedSessionId ? session : undefined;
  const announcedId = announced?.id;
  useEffect(() => {
    if (announcedId === undefined) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setDismissedId(announcedId);
    }, toastDurationMilliseconds);
    return () => {
      clearTimeout(timer);
    };
  }, [announcedId]);
  const visible = announced !== undefined && announced.id !== dismissedId;
  return {
    toasts: visible ? sessionToasts(announced) : [],
    dismiss: () => {
      setDismissedId(announcedId);
    },
  };
}
