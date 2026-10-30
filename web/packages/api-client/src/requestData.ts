import { toApiError, toApiErrorFromException, toEmptyBodyError } from "./apiError";

const noContentStatus = 204;

interface SuccessResult<Data> {
  readonly data: Data;
  readonly error?: never;
  readonly response: Response;
}

interface FailureResult {
  readonly data?: never;
  readonly error: unknown;
  readonly response: Response;
}

/** Structural twin of openapi-fetch's `FetchResponse`, so any client call can be passed. */
export type RequestResult<Data> = SuccessResult<Data> | FailureResult;

/**
 * Awaits an openapi-fetch call and returns its data, or throws an `ApiError` for every
 * failure: non-2xx responses, network errors, unreadable bodies and a missing body on any 2xx
 * except 204 No Content (which resolves to `undefined`). Abort errors are rethrown
 * unchanged. Fits TanStack Query's `queryFn`, which expects a rejecting promise.
 *
 * Endpoints that answer a failure status with a typed non-problem body (for example 503 from
 * `GET /api/v1/health`) need the raw client result instead, because that body is data.
 */
export async function requestData<Data>(request: Promise<RequestResult<Data>>): Promise<Data> {
  let result: RequestResult<Data>;
  try {
    result = await request;
  } catch (cause) {
    throw toApiErrorFromException(cause);
  }
  if (!isSuccess(result)) {
    throw toApiError(result);
  }
  // openapi-fetch yields `data: undefined` for an empty body (for example `Content-Length: 0`)
  // instead of failing. Only 204 promises no body; anywhere else the caller would read the
  // fields of `undefined`.
  if (result.data === undefined && result.response.status !== noContentStatus) {
    throw toEmptyBodyError(result.response);
  }
  return result.data;
}

// openapi-fetch decides success by `response.ok`, so the same check selects the branch.
function isSuccess<Data>(result: RequestResult<Data>): result is SuccessResult<Data> {
  return result.response.ok;
}
