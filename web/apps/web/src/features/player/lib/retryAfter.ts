// Reads the `Retry-After` header the server sends with 503 `session_limit_reached` and
// `stream_not_ready` (api/openapi.yaml: integer seconds, at least 1).

/** Name of the header, as the API contract spells it. */
export const retryAfterHeader = "Retry-After";

/**
 * Longest delay taken from a server, in seconds. A larger value (a bug or a hostile answer)
 * would leave the player waiting or show an absurd retry hint; five minutes is the cap.
 */
export const maxRetryAfterSeconds = 300;

const delaySecondsPattern = /^\d+$/;

/**
 * The delay of a `Retry-After` value in seconds, at most {@link maxRetryAfterSeconds}, or
 * `undefined` when it is missing or not a positive whole number. HTTP dates are not accepted:
 * the API contract only sends seconds.
 */
export function parseRetryAfter(value: string | null | undefined): number | undefined {
  const trimmed = value?.trim() ?? "";
  if (!delaySecondsPattern.test(trimmed)) {
    return undefined;
  }
  const seconds = Number(trimmed);
  return seconds > 0 ? Math.min(seconds, maxRetryAfterSeconds) : undefined;
}

/** The part of an `XMLHttpRequest` that answers headers (hls.js' default loader). */
interface RequestWithHeaders {
  getResponseHeader(name: string): string | null;
}

/** The part of a `Response` that answers headers (hls.js' fetch loader). */
interface ResponseWithHeaders {
  readonly headers: { get(name: string): string | null };
}

function isRequestWithHeaders(details: unknown): details is RequestWithHeaders {
  return (
    typeof details === "object" &&
    details !== null &&
    typeof (details as Partial<RequestWithHeaders>).getResponseHeader === "function"
  );
}

function isResponseWithHeaders(details: unknown): details is ResponseWithHeaders {
  if (typeof details !== "object" || details === null) {
    return false;
  }
  const { headers } = details as { readonly headers?: unknown };
  return (
    typeof headers === "object" &&
    headers !== null &&
    typeof (headers as Partial<ResponseWithHeaders["headers"]>).get === "function"
  );
}

/**
 * The `Retry-After` delay of a failed hls.js request in seconds, read from its network details
 * (an `XMLHttpRequest` or a `Response`), or `undefined` when there is none.
 */
export function retryAfterFromNetworkDetails(details: unknown): number | undefined {
  if (isRequestWithHeaders(details)) {
    return parseRetryAfter(details.getResponseHeader(retryAfterHeader));
  }
  if (isResponseWithHeaders(details)) {
    return parseRetryAfter(details.headers.get(retryAfterHeader));
  }
  return undefined;
}
