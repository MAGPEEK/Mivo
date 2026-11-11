import type { Me, Session, SetupState } from "@mivo/api-client";
import type { QueryClient } from "@tanstack/react-query";

import { authQueryKey, meQueryKey, setupStateQueryKey } from "../api/authApi";

function isAuthQuery(queryKey: readonly unknown[]): boolean {
  return queryKey[0] === authQueryKey[0];
}

/**
 * Forgets everything the previous session loaded and checks the session again. Catalog data
 * of one account must never show for the next, so all other queries are removed; the auth
 * queries are reset, which puts the guard into `loading` until the server answered.
 */
export function resetSession(queryClient: QueryClient): void {
  queryClient.removeQueries({ predicate: (query) => !isAuthQuery(query.queryKey) });
  void queryClient.resetQueries({ queryKey: authQueryKey });
}

/** Stores a new session, so the guard moves on without asking the server again. */
export function storeSession(queryClient: QueryClient, session: Session): void {
  const setupDone: SetupState = { required: false };
  queryClient.setQueryData(setupStateQueryKey, setupDone);
  queryClient.setQueryData(meQueryKey, session.me);
}

/**
 * Stores the account after its profile changed (selected or left) and drops everything the
 * previous profile loaded, so every page loads again with the new profile's limits. The account
 * is stored first: the guard then leaves the page before a query could refetch.
 */
export function storeProfileChange(queryClient: QueryClient, me: Me): void {
  queryClient.setQueryData(meQueryKey, me);
  queryClient.removeQueries({ predicate: (query) => !isAuthQuery(query.queryKey) });
}

/**
 * Loads the account again after its name or its profiles changed, so the profile menu and the
 * picker show the change. Without a stored account (signed out, or a server without accounts)
 * nothing happens: refetching a failed check would put the session gate back into `loading`.
 */
export async function refreshAccount(queryClient: QueryClient): Promise<void> {
  if (queryClient.getQueryData(meQueryKey) !== undefined) {
    await queryClient.invalidateQueries({ queryKey: meQueryKey });
  }
}

/** The name of the account signed in last, for prefilling the sign-in form. */
export function cachedAccountName(queryClient: QueryClient): string | undefined {
  return queryClient.getQueryData<Me>(meQueryKey)?.account.name;
}
