import {
  ApiError,
  noResponseStatus,
  requestData,
  type ApiClient,
  type Me,
  type Session,
  type SetupState,
} from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Prefix of every auth cache key; resetting it re-checks the whole session. */
export const authQueryKey = ["auth"] as const;

/** Cache key of `GET /api/v1/setup`. */
export const setupStateQueryKey = [...authQueryKey, "setup"] as const;

/** Cache key of `GET /api/v1/me`. */
export const meQueryKey = [...authQueryKey, "me"] as const;

// The web client always signs in as "web": the server then sets the HttpOnly cookie and never
// hands out a token the page could leak (task M7-30 note).
const webClient = "web";

const maxNetworkRetries = 1;

// Answers are final: a 401, 403 or 501 does not change on a retry. Only a lost connection is
// tried once more, to ride out a server restart.
function retryOnlyNetwork(failureCount: number, error: unknown): boolean {
  const answered = error instanceof ApiError && error.status !== noResponseStatus;
  return !answered && failureCount < maxNetworkRetries;
}

/** Query options for whether the server still waits for its first account. */
export function setupStateQueryOptions(client: ApiClient) {
  return queryOptions<SetupState>({
    queryKey: setupStateQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/setup", { signal })),
    // Setup happens once; the setup and sign-in flows update the cache themselves.
    staleTime: Infinity,
    retry: retryOnlyNetwork,
  });
}

/** Query options for the signed-in account; a 401 means "signed out". */
export function meQueryOptions(client: ApiClient) {
  return queryOptions<Me>({
    queryKey: meQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/me", { signal })),
    retry: retryOnlyNetwork,
  });
}

/** Credentials typed into the sign-in or setup form. */
export interface Credentials {
  readonly name: string;
  readonly password: string;
}

/** Signs in; the response sets the session cookie. */
export function signIn(client: ApiClient, credentials: Credentials): Promise<Session> {
  return requestData(
    client.POST("/api/v1/auth/session", { body: { ...credentials, client: webClient } }),
  );
}

/** Creates the first admin account and signs in with it. */
export function completeSetup(client: ApiClient, credentials: Credentials): Promise<Session> {
  return requestData(client.POST("/api/v1/setup", { body: { ...credentials, client: webClient } }));
}

/**
 * Asks the server whether this peer may run the setup, without creating anything: an empty
 * name is always refused, and the local-peer rule answers before the input check (ADR 0032 §6).
 * Resolves to the refusal the server sent.
 */
export async function probeSetupAccess(client: ApiClient): Promise<ApiError | undefined> {
  try {
    await completeSetup(client, { name: "", password: "" });
  } catch (error) {
    if (error instanceof ApiError) {
      return error;
    }
    throw error;
  }
  return undefined;
}

/** Ends the session of this browser. */
export async function signOut(client: ApiClient): Promise<void> {
  await requestData(client.DELETE("/api/v1/auth/session"));
}

/** Request of {@link changePassword}. */
export interface PasswordChangeRequest {
  readonly currentPassword: string;
  readonly newPassword: string;
}

/** Changes the account's own password; the account's other sessions end. */
export async function changePassword(
  client: ApiClient,
  request: PasswordChangeRequest,
): Promise<void> {
  await requestData(client.PUT("/api/v1/me/password", { body: request }));
}
