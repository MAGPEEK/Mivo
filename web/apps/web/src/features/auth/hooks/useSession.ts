import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { meQueryOptions, setupStateQueryOptions } from "../api/authApi";
import { deriveSession, type SessionState } from "../lib/sessionStatus";

/**
 * The session of this browser from `GET /setup` and `GET /me`: `setupRequired`, `signedOut`,
 * `needsProfile` or `ready` (plus `loading` until both answered). A server without accounts
 * (`501`) counts as `ready` with `authAvailable` false.
 */
export function useSession(): SessionState {
  const client = useApiClient();
  const setup = useQuery(setupStateQueryOptions(client));
  const me = useQuery(meQueryOptions(client));
  return deriveSession(setup, me);
}

// A component below the gate must not check again on mount: on a failed check (a `501`, an
// unreachable server) a new observer refetches and turns the session back into `loading`, the
// gate unmounts the page with the component, the check fails again – an endless loop.
const withoutMountCheck = { refetchOnMount: false } as const;

/**
 * The session as the gate last saw it, for components below the session gate such as the
 * profile menu and the picker. Unlike {@link useSession} it never starts a check on mount.
 */
export function useSessionSnapshot(): SessionState {
  const client = useApiClient();
  const setup = useQuery({ ...setupStateQueryOptions(client), ...withoutMountCheck });
  const me = useQuery({ ...meQueryOptions(client), ...withoutMountCheck });
  return deriveSession(setup, me);
}
