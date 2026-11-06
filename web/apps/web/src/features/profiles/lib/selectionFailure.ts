import { ApiError, clientErrorCodes } from "@mivo/api-client";

/** Why selecting a profile failed, as the picker tells it. */
export type SelectionFailure =
  | { readonly kind: "wrongPin" }
  | { readonly kind: "locked"; readonly seconds: number }
  | { readonly kind: "network" }
  | { readonly kind: "generic" };

const forbiddenStatus = 403;
const tooManyAttemptsStatus = 429;
const pinInvalidCode = "pin_invalid";

/** The lockout of ADR 0032 §4, used when a `429` names no wait. */
export const defaultLockoutSeconds = 300;

/** Classifies a failed `PUT /me/profile` (api/openapi.yaml). */
export function selectionFailure(error: unknown): SelectionFailure {
  if (!(error instanceof ApiError)) {
    return { kind: "generic" };
  }
  if (error.status === forbiddenStatus && error.code === pinInvalidCode) {
    return { kind: "wrongPin" };
  }
  if (error.status === tooManyAttemptsStatus) {
    return { kind: "locked", seconds: error.retryAfterSeconds ?? defaultLockoutSeconds };
  }
  return error.code === clientErrorCodes.networkError ? { kind: "network" } : { kind: "generic" };
}
