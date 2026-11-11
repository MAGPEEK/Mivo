import {
  ApiError,
  noResponseStatus,
  requestData,
  type ApiClient,
  type Profile,
  type User,
  type UserCreate,
  type UserUpdate,
} from "@mivo/api-client";
import { queryOptions, type QueryClient } from "@tanstack/react-query";

import { refreshAccount } from "../../auth";

/** Prefix of every user administration cache key. */
export const usersQueryKey = ["users"] as const;

/**
 * Loads everything that shows accounts or profiles again after one changed: the user
 * administration and the signed-in account, whose own name or profiles may be the change.
 */
export async function refreshAccounts(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: usersQueryKey }),
    refreshAccount(queryClient),
  ]);
}

const maxNetworkRetries = 1;

// A 403 or 501 does not change on a retry; only a lost connection is tried once more.
function retryOnlyNetwork(failureCount: number, error: unknown): boolean {
  const answered = error instanceof ApiError && error.status !== noResponseStatus;
  return !answered && failureCount < maxNetworkRetries;
}

/** Query options for every account, ordered by name. */
export function usersQueryOptions(client: ApiClient) {
  return queryOptions<readonly User[]>({
    queryKey: [...usersQueryKey, "list"] as const,
    queryFn: async ({ signal }) =>
      (await requestData(client.GET("/api/v1/users", { signal }))).users,
    retry: retryOnlyNetwork,
  });
}

/** Query options for the profiles of one account, the primary profile first. */
export function userProfilesQueryOptions(client: ApiClient, userId: string) {
  return queryOptions<readonly Profile[]>({
    queryKey: [...usersQueryKey, userId, "profiles"] as const,
    queryFn: async ({ signal }) =>
      (
        await requestData(
          client.GET("/api/v1/users/{userId}/profiles", { params: { path: { userId } }, signal }),
        )
      ).profiles,
    retry: retryOnlyNetwork,
  });
}

/** Creates an account with its primary profile. */
export function createUser(client: ApiClient, body: UserCreate): Promise<User> {
  return requestData(client.POST("/api/v1/users", { body }));
}

/** Request of {@link updateUser}. */
export interface UpdateUserRequest {
  readonly userId: string;
  readonly body: UserUpdate;
}

/** Changes an account; omitted properties stay as they are. */
export function updateUser(client: ApiClient, request: UpdateUserRequest): Promise<User> {
  return requestData(
    client.PATCH("/api/v1/users/{userId}", {
      params: { path: { userId: request.userId } },
      body: request.body,
    }),
  );
}

/** Request of {@link resetUserPassword}. */
export interface PasswordResetRequest {
  readonly userId: string;
  readonly password: string;
}

/** Sets a new password for an account; its sessions end. Resolves on 204. */
export async function resetUserPassword(
  client: ApiClient,
  request: PasswordResetRequest,
): Promise<void> {
  await requestData(
    client.PUT("/api/v1/users/{userId}/password", {
      params: { path: { userId: request.userId } },
      body: { password: request.password },
    }),
  );
}

/** Deletes an account with its profiles, their watch state and its sessions. */
export async function deleteUser(client: ApiClient, userId: string): Promise<void> {
  await requestData(client.DELETE("/api/v1/users/{userId}", { params: { path: { userId } } }));
}
