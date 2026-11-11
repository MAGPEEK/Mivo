/** Path of the own account page with the account's profiles; the profile menu links here. */
export const ownAccountPath = "/settings/account";

/** Value of the `profile` search parameter that opens the editor for a new profile. */
export const newProfileTarget = "new";

/** The own account page with "Create managed profile" open; "Add profile" in the picker. */
export const addProfileHref = `${ownAccountPath}?profile=${newProfileTarget}`;

/** Search of the own account page. */
export interface AccountSearch {
  /** The profile whose editor is open: a profile ID, or {@link newProfileTarget}. */
  readonly profile?: string | undefined;
}

// A UUID has 36 characters; anything much longer is not an ID of this app.
const maxProfileTargetLength = 64;

/** Validates the search of the own account page: anything but a short string is dropped. */
export function validateAccountSearch(search: Record<string, unknown>): AccountSearch {
  const profile = search["profile"];
  const valid =
    typeof profile === "string" && profile !== "" && profile.length <= maxProfileTargetLength;
  return { profile: valid ? profile : undefined };
}
