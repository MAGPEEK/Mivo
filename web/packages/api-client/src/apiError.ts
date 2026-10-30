/**
 * Codes the client assigns itself when the server sent no usable problem details. They share
 * the namespace of the server's `Problem.code`, so the apps translate both the same way.
 */
export const clientErrorCodes = {
  /** The request never produced an HTTP response (offline, DNS, refused, CORS, ...). */
  networkError: "network_error",
  /** The server answered, but not with the body the contract promises. */
  unexpectedResponse: "unexpected_response",
} as const;

/**
 * `ApiError.status` when no HTTP status is known. Zero mirrors the status of a network error
 * in the Fetch standard (`Response.error()`).
 */
export const noResponseStatus = 0;

/** Fields of an {@link ApiError}; they follow RFC 9457 problem details. */
export interface ApiErrorFields {
  /** HTTP status, or {@link noResponseStatus} when no usable response arrived. */
  readonly status: number;
  /** Short English summary; for logs, not for display. */
  readonly title: string;
  /** Machine-readable code the apps translate into a user-facing message. */
  readonly code: string;
  /** English explanation of this occurrence; for logs, not for display. */
  readonly detail?: string | undefined;
}

/** The single error type every failed API call is normalized to. */
export class ApiError extends Error implements ApiErrorFields {
  override readonly name = "ApiError";
  readonly status: number;
  readonly title: string;
  readonly code: string;
  readonly detail: string | undefined;

  /**
   * @param options `cause` is for diagnostics only: engines without error causes (webOS 6
   *   runs Chromium 79) ignore it, so no app logic may depend on it.
   */
  constructor(fields: ApiErrorFields, options?: ErrorOptions) {
    super(fields.title, options);
    this.status = fields.status;
    this.title = fields.title;
    this.code = fields.code;
    this.detail = fields.detail;
  }
}

/** The failure branch of an openapi-fetch result: the parsed body (if any) and the response. */
export interface ErrorResult {
  readonly error?: unknown;
  readonly response: Response;
}

const problemMediaType = "application/problem+json";
const unexpectedResponseTitle = "Unexpected response";

/**
 * Converts a non-2xx openapi-fetch result into an {@link ApiError}. Problem details are used
 * when the server sent them; any other body yields `unexpected_response`. The caller
 * guarantees `!response.ok`; this function does not check it again.
 */
export function toApiError(result: ErrorResult): ApiError {
  const { error: body, response } = result;
  // RFC 9457 makes the HTTP status authoritative over the advisory `status` member.
  if (hasProblemMediaType(response) && isProblemSummary(body)) {
    return new ApiError({
      status: response.status,
      title: body.title,
      code: body.code,
      detail: body.detail,
    });
  }
  return new ApiError({
    status: response.status,
    title: unexpectedResponseTitle,
    code: clientErrorCodes.unexpectedResponse,
    detail: `HTTP status ${String(response.status)} without problem details.`,
  });
}

/**
 * Builds the error for a 2xx response without a body although the contract promises one.
 * The server answered, so this is `unexpected_response` with the received status.
 */
export function toEmptyBodyError(response: Response): ApiError {
  return new ApiError({
    status: response.status,
    title: unexpectedResponseTitle,
    code: clientErrorCodes.unexpectedResponse,
    detail: `HTTP status ${String(response.status)} without a body.`,
  });
}

/**
 * Converts what an openapi-fetch call threw into an {@link ApiError}. Abort errors are
 * returned unchanged so callers (for example TanStack Query) still recognise cancellation.
 */
export function toApiErrorFromException(cause: unknown): Error {
  if (isAbortError(cause)) {
    return cause;
  }
  // openapi-fetch only throws a SyntaxError when a 2xx body is not valid JSON: the server
  // answered, so reporting a network failure would send the user to check the connection.
  if (cause instanceof SyntaxError) {
    return new ApiError(
      {
        status: noResponseStatus,
        title: unexpectedResponseTitle,
        code: clientErrorCodes.unexpectedResponse,
        detail: cause.message,
      },
      { cause },
    );
  }
  return new ApiError(
    {
      status: noResponseStatus,
      title: "Network request failed",
      code: clientErrorCodes.networkError,
      detail: cause instanceof Error ? cause.message : undefined,
    },
    { cause },
  );
}

function isAbortError(cause: unknown): cause is Error {
  // DOMException inherits from Error in browsers and Node, so one check covers both.
  return cause instanceof Error && cause.name === "AbortError";
}

function hasProblemMediaType(response: Response): boolean {
  const contentType = response.headers.get("Content-Type") ?? "";
  const [mediaType = ""] = contentType.split(";");
  return mediaType.trim().toLowerCase() === problemMediaType;
}

/** The problem members an {@link ApiError} needs; the body is untrusted, so each is checked. */
interface ProblemSummary {
  readonly title: string;
  readonly code: string;
  readonly detail?: string | undefined;
}

function isProblemSummary(body: unknown): body is ProblemSummary {
  if (typeof body !== "object" || body === null) {
    return false;
  }
  const { title, code, detail } = body as Record<string, unknown>;
  return (
    typeof title === "string" &&
    typeof code === "string" &&
    (detail === undefined || typeof detail === "string")
  );
}
