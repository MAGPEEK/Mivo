import type { Me, ProfileSummary } from "@mivo/api-client";

import { maxManagedProfiles } from "../../users";

/** Whether choosing `profile` asks for its PIN first. */
export function needsPin(profile: ProfileSummary): boolean {
  return profile.hasPin;
}

/**
 * The initial "Remember on this device" choice (US-03): off for a profile with a PIN, so a
 * child cannot reopen it hours later; on otherwise, matching the server default.
 */
export function rememberByDefault(profile: ProfileSummary): boolean {
  return !profile.hasPin;
}

/** The account's primary profile, the one the account password belongs to. */
export function primaryProfile(me: Me): ProfileSummary | undefined {
  return me.profiles.find((profile) => profile.isPrimary);
}

function managedCount(me: Me): number {
  return me.profiles.filter((profile) => !profile.isPrimary).length;
}

/** Whether the picker offers "Add profile": below the managed-profile limit. */
export function canAddProfile(me: Me): boolean {
  return primaryProfile(me) !== undefined && managedCount(me) < maxManagedProfiles;
}

/**
 * Whether the profile menu asks the owner to set a PIN (ADR 0032 §4): on the primary profile
 * without a PIN once managed profiles exist, else a managed profile could switch to it freely.
 */
export function suggestsOwnPin(me: Me): boolean {
  const current = me.currentProfile;
  return current !== null && current.isPrimary && !current.hasPin && managedCount(me) > 0;
}

/** Whether the profile may manage the account's profiles: only the primary profile (ADR 0032 §1). */
export function canManageProfiles(profile: ProfileSummary): boolean {
  return profile.isPrimary;
}

/** Whether the profile may change the account password: only the primary profile owns it. */
export function canChangePassword(profile: ProfileSummary): boolean {
  return profile.isPrimary;
}
