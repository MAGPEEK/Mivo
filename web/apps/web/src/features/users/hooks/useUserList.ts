import type { Profile, User } from "@mivo/api-client";
import { useQueries, useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { userProfilesQueryOptions, usersQueryOptions } from "../api/usersApi";
import { isForbidden, isNotImplemented } from "../lib/userProblems";

/** One account of the list with the managed profiles nested under it (US-20). */
export interface UserEntry {
  readonly user: User;
  /** Managed profiles by name; empty while they load or when the account has none. */
  readonly managedProfiles: readonly Profile[];
  /** True when the account has managed profiles but they could not be loaded. */
  readonly profilesFailed: boolean;
}

/**
 * What the users page shows: `unavailable` while the server does not serve user
 * administration yet (501), `forbidden` when it refuses the viewer, `error` for any other
 * failure.
 */
export type UserListState =
  | { readonly status: "loading" }
  | { readonly status: "unavailable" }
  | { readonly status: "forbidden" }
  | { readonly status: "error"; readonly retry: () => void }
  | { readonly status: "ready"; readonly entries: readonly UserEntry[] };

// Every account has its primary profile; only accounts with more have managed ones to load.
const primaryProfileOnly = 1;

/** Loads the accounts and, for those with managed profiles, their profiles. */
export function useUserList(): UserListState {
  const client = useApiClient();
  const users = useQuery(usersQueryOptions(client));
  const withManaged = (users.data ?? []).filter((user) => user.profileCount > primaryProfileOnly);
  const profiles = useQueries({
    queries: withManaged.map((user) => userProfilesQueryOptions(client, user.id)),
  });
  if (users.isPending) {
    return { status: "loading" };
  }
  if (users.isError) {
    if (isNotImplemented(users.error)) {
      return { status: "unavailable" };
    }
    return isForbidden(users.error)
      ? { status: "forbidden" }
      : { status: "error", retry: () => void users.refetch() };
  }
  const queryByUser = new Map(withManaged.map((user, index) => [user.id, profiles[index]]));
  const entries = users.data.map((user) => {
    const query = queryByUser.get(user.id);
    return {
      user,
      managedProfiles: (query?.data ?? []).filter((profile) => !profile.isPrimary),
      profilesFailed: query?.isError === true,
    };
  });
  return { status: "ready", entries };
}
