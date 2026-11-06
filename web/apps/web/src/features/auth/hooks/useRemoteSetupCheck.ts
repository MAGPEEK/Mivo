import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";

import { useApiClient } from "../../../shared/api";
import { probeSetupAccess } from "../api/authApi";
import { homePath, setupPath } from "../lib/authPaths";
import { answers, generalFailure, matches, type GeneralFailure } from "../lib/authErrors";
import { resetSession } from "./sessionCache";

/** What "Check again" found out, when it did not move on. */
export type RemoteCheckResult = "stillRemote" | GeneralFailure;

/** State and action of "Check again" on the "Only from the local network" page (AU-03). */
export interface RemoteSetupCheck {
  readonly check: () => void;
  readonly pending: boolean;
  readonly result: RemoteCheckResult | undefined;
}

/**
 * Asks the server again whether this connection may run the setup. A local connection goes
 * back to the form, a finished setup to sign-in (through the guard); a remote one stays.
 */
export function useRemoteSetupCheck(): RemoteSetupCheck {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const mutation = useMutation({
    mutationFn: () => probeSetupAccess(client),
    onSuccess: (refusal) => {
      if (matches(refusal, answers.invalid)) {
        void navigate({ href: setupPath });
      } else if (matches(refusal, answers.conflict)) {
        resetSession(queryClient);
      } else if (matches(refusal, answers.notImplemented)) {
        void navigate({ href: homePath });
      }
    },
  });
  return {
    check: () => {
      if (!mutation.isPending) {
        mutation.mutate();
      }
    },
    pending: mutation.isPending,
    result: remoteCheckResult(mutation.data, mutation.error, mutation.isSuccess),
  };
}

function remoteCheckResult(
  refusal: unknown,
  error: Error | null,
  settled: boolean,
): RemoteCheckResult | undefined {
  if (error !== null) {
    return generalFailure(error);
  }
  if (!settled) {
    return undefined;
  }
  return matches(refusal, answers.forbidden) ? "stillRemote" : generalFailure(refusal);
}
