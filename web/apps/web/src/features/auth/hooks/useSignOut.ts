import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";

import { useApiClient } from "../../../shared/api";
import { signOut } from "../api/authApi";
import { signInPath } from "../lib/authPaths";
import { answers, matches } from "../lib/authErrors";
import { authHref } from "../lib/authSearch";
import { resetSession } from "./sessionCache";

/** The "Sign out" action for the profile menu (SH-07, M7-31). */
export interface SignOutAction {
  readonly signOut: () => void;
  readonly pending: boolean;
  /** True when the server could not end the session; this device is still signed in. */
  readonly failed: boolean;
}

// No session to end (401) or no accounts at all (501): the device is signed out either way.
function endsSessionAnyway(error: Error): boolean {
  return matches(error, answers.unauthorized) || matches(error, answers.notImplemented);
}

/**
 * Ends the session of this browser, forgets all cached data and opens the sign-in page with
 * "You have signed out" (AU-05). Other devices stay signed in.
 */
export function useSignOut(): SignOutAction {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const finish = () => {
    // Cache first: with the old session still cached the guard would leave the sign-in page.
    resetSession(queryClient);
    void navigate({ href: authHref(signInPath, { reason: "signed-out" }), replace: true });
  };
  const mutation = useMutation({
    mutationFn: () => signOut(client),
    onSuccess: finish,
    onError: (error) => {
      if (endsSessionAnyway(error)) {
        finish();
      }
    },
  });
  return {
    signOut: () => {
      if (!mutation.isPending) {
        mutation.mutate();
      }
    },
    pending: mutation.isPending,
    failed: mutation.isError && !endsSessionAnyway(mutation.error),
  };
}
