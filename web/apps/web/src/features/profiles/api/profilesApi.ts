import { requestData, type ApiClient, type Me } from "@mivo/api-client";

/** A profile choice for `PUT /me/profile`. */
export interface ProfileChoice {
  readonly profileId: string;
  /** The 4-digit PIN; only for a profile that has one. */
  readonly pin?: string | undefined;
  /** Keep the selection on this device; omitted, the server decides by the PIN. */
  readonly remember?: boolean | undefined;
}

/** Selects a profile for this session and returns the account with it. */
export function selectProfile(client: ApiClient, choice: ProfileChoice): Promise<Me> {
  const { profileId, pin, remember } = choice;
  return requestData(
    client.PUT("/api/v1/me/profile", {
      body: {
        profileId,
        ...(pin === undefined ? {} : { pin }),
        ...(remember === undefined ? {} : { remember }),
      },
    }),
  );
}

/** Leaves the selected profile ("Switch profile"); the session stays signed in. */
export async function clearProfileSelection(client: ApiClient): Promise<void> {
  await requestData(client.DELETE("/api/v1/me/profile"));
}
