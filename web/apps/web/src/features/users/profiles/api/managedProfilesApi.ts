import {
  requestData,
  type ApiClient,
  type Profile,
  type ProfileCreate,
  type ProfileUpdate,
} from "@mivo/api-client";

/** Request of {@link createProfile}. */
export interface CreateProfileRequest {
  /** The account that gets the profile. */
  readonly userId: string;
  readonly body: ProfileCreate;
}

/** Adds a managed profile to an account. */
export function createProfile(client: ApiClient, request: CreateProfileRequest): Promise<Profile> {
  return requestData(
    client.POST("/api/v1/users/{userId}/profiles", {
      params: { path: { userId: request.userId } },
      body: request.body,
    }),
  );
}

/** Request of {@link updateProfile}. */
export interface UpdateProfileRequest {
  readonly profileId: string;
  readonly body: ProfileUpdate;
}

/** Changes a profile; omitted properties stay as they are, a `pin` of null removes the PIN. */
export function updateProfile(client: ApiClient, request: UpdateProfileRequest): Promise<Profile> {
  return requestData(
    client.PATCH("/api/v1/profiles/{profileId}", {
      params: { path: { profileId: request.profileId } },
      body: request.body,
    }),
  );
}

/** Deletes a managed profile with its watch state. */
export async function deleteProfile(client: ApiClient, profileId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/profiles/{profileId}", { params: { path: { profileId } } }),
  );
}
