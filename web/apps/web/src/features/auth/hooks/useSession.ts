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
