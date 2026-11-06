import { ApiError, clientErrorCodes } from "@mivo/api-client";

/** The general failure texts of the auth forms. */
export type GeneralFailure = "network" | "generic";

/** Shortest wait when a 429 names none: the first throttling step of ADR 0032 §5. */
export const defaultThrottleSeconds = 60;

/** Whether `error` is an API answer with `status` and, when given, `code`. */
export function isAnswer(error: unknown, status: number, code?: string): error is ApiError {
  return (
    error instanceof ApiError &&
    error.status === status &&
    (code === undefined || error.code === code)
  );
}

/** The general text for a failure no form handles itself. */
export function generalFailure(error: unknown): GeneralFailure {
  return error instanceof ApiError && error.code === clientErrorCodes.networkError
    ? "network"
    : "generic";
}

/** The wait a `429` asks for, in seconds. */
export function throttleSeconds(error: ApiError): number {
  return error.retryAfterSeconds ?? defaultThrottleSeconds;
}

/** HTTP statuses and problem codes the auth forms react to (api/openapi.yaml). */
export const answers = {
  invalid: { status: 400, code: "invalid" },
  unauthorized: { status: 401, code: undefined },
  setupRequired: { status: 401, code: "setup_required" },
  wrongCurrentPassword: { status: 403, code: "invalid_credentials" },
  forbidden: { status: 403, code: "forbidden" },
  conflict: { status: 409, code: "conflict" },
  tooManyAttempts: { status: 429, code: undefined },
  notImplemented: { status: 501, code: undefined },
} as const;

/** Shorthand of {@link isAnswer} for an entry of {@link answers}. */
export function matches(
  error: unknown,
  answer: { readonly status: number; readonly code: string | undefined },
): error is ApiError {
  return isAnswer(error, answer.status, answer.code);
}
