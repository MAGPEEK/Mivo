import type { Profile } from "@mivo/api-client";
import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../../shared/api";
import { useSessionSnapshot } from "../../../auth";
import { userProfilesQueryOptions } from "../../api/usersApi";
import { isForbidden } from "../../lib/userProblems";
import type { ProfileOwner } from "../lib/profileDraft";

/**
 * What the own account page shows: `unavailable` on a server without accounts,
 * `primaryRequired` while a managed profile is selected (only the primary profile manages the
 * account's profiles, ADR 0032 §1), `forbidden` when the server refuses anyway.
 */
export type OwnProfilesState =
  | { readonly status: "unavailable" }
  | { readonly status: "primaryRequired" }
  | { readonly status: "loading" }
  | { readonly status: "forbidden" }
  | { readonly status: "error"; readonly retry: () => void }
  | {
      readonly status: "ready";
      readonly owner: ProfileOwner;
      /** The primary profile first, then the managed ones by name. */
      readonly profiles: readonly Profile[];
    };

/** Loads the profiles of the signed-in account for its owner. */
export function useOwnProfiles(): OwnProfilesState {
  const client = useApiClient();
  const { me } = useSessionSnapshot();
  const account = me?.account;
  const manages = me?.currentProfile?.isPrimary === true;
  const query = useQuery({
    ...userProfilesQueryOptions(client, account?.id ?? ""),
    enabled: account !== undefined && manages,
  });
  if (account === undefined) {
    return { status: "unavailable" };
  }
  if (!manages) {
    return { status: "primaryRequired" };
  }
  if (query.isPending) {
    return { status: "loading" };
  }
  if (query.isError) {
    return isForbidden(query.error)
      ? { status: "forbidden" }
      : { status: "error", retry: () => void query.refetch() };
  }
  // The primary profile carries the account's own libraries: the most a managed one can get.
  const primary = query.data.find((profile) => profile.isPrimary);
  const owner: ProfileOwner = {
    id: account.id,
    name: account.name,
    libraryAccess: primary?.libraryAccess ?? { all: true },
  };
  return { status: "ready", owner, profiles: query.data };
}
