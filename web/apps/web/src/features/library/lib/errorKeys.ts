import { ApiError, clientErrorCodes } from "@mivo/api-client";

// Problem and scan codes the library feature translates. Unknown codes fall back to a generic
// text: servers of the same major version may send codes this client does not know yet.
const errorKeys = {
  root_unavailable: "errors.rootUnavailable",
  prober_unavailable: "errors.proberUnavailable",
  catalog_failed: "errors.catalogFailed",
  scan_canceled: "errors.scanCanceled",
  scan_failed: "errors.scanFailed",
  probe_failed: "errors.probeFailed",
  probe_timed_out: "errors.probeTimedOut",
  not_found: "errors.notFound",
  unavailable: "errors.unavailable",
  [clientErrorCodes.networkError]: "errors.networkError",
} as const;

/** Translation key of an error text in the library namespace. */
export type ErrorKey = (typeof errorKeys)[keyof typeof errorKeys] | "errors.generic";

/** The library-namespace key that explains `code`; `errors.generic` for unknown codes. */
export function errorKeyForCode(code: string | undefined): ErrorKey {
  return code !== undefined && Object.hasOwn(errorKeys, code)
    ? errorKeys[code as keyof typeof errorKeys]
    : "errors.generic";
}

/** The machine-readable code of a failed request; undefined for anything but an `ApiError`. */
export function errorCodeOf(error: unknown): string | undefined {
  return error instanceof ApiError ? error.code : undefined;
}
