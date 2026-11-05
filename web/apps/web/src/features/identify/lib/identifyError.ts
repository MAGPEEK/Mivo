import { ApiError, clientErrorCodes } from "@mivo/api-client";

/** What went wrong with an identify, remove or refresh call, as the UI explains it. */
export type IdentifyErrorKind =
  | "providerOffline"
  | "onlineDisabled"
  | "notSupported"
  | "forbidden"
  | "invalid"
  | "notFound"
  | "networkError"
  | "generic";

const codeKinds = new Map<string, IdentifyErrorKind>([
  ["provider_unavailable", "providerOffline"],
  ["online_disabled", "onlineDisabled"],
  // Servers before online metadata (M6-13) answer the metadata routes with 501.
  ["not_implemented", "notSupported"],
  ["forbidden", "forbidden"],
  ["invalid", "invalid"],
  ["not_found", "notFound"],
  [clientErrorCodes.networkError, "networkError"],
]);

const statusKinds = new Map<number, IdentifyErrorKind>([
  [400, "invalid"],
  [403, "forbidden"],
  [404, "notFound"],
  [409, "onlineDisabled"],
  [501, "notSupported"],
  [503, "providerOffline"],
]);

const transientKinds: ReadonlySet<IdentifyErrorKind> = new Set(["providerOffline", "networkError"]);

/** Whether trying the same call again later may succeed, so the UI offers a retry. */
export function isRetryable(kind: IdentifyErrorKind): boolean {
  return transientKinds.has(kind);
}

/**
 * Classifies a failed metadata call. The problem code wins over the status; the status covers
 * answers without problem details, for example a 503 of a proxy while the provider is down.
 */
export function identifyErrorKind(error: unknown): IdentifyErrorKind {
  if (!(error instanceof ApiError)) {
    return "generic";
  }
  return codeKinds.get(error.code) ?? statusKinds.get(error.status) ?? "generic";
}
