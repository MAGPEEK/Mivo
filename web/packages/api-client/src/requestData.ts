import { toApiError, toApiErrorFromException } from "./apiError";

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
 * failure: non-2xx responses, network errors and unreadable bodies. Abort errors are rethrown
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
  return result.data;
}

// openapi-fetch decides success by `response.ok`, so the same check selects the branch.
function isSuccess<Data>(result: RequestResult<Data>): result is SuccessResult<Data> {
  return result.response.ok;
}
