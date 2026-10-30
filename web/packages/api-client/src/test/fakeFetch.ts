/** A fetch replacement that records requests and never touches the network. */
export interface FakeFetch {
  readonly fetch: (request: Request) => Promise<Response>;
  readonly requests: readonly Request[];
}

/** Base URL used by tests that do not exercise the default. */
export const testBaseUrl = "http://server.test";

/** Answers every request with a fresh response built by `createResponse`. */
export function respondWith(createResponse: () => Response): FakeFetch {
  const requests: Request[] = [];
  return {
    requests,
    fetch: (request) => {
      requests.push(request);
      return Promise.resolve(createResponse());
    },
  };
}

/** Rejects every request with `cause`, like fetch does when no response arrives. */
export function rejectWith(cause: unknown): FakeFetch {
  const requests: Request[] = [];
  return {
    requests,
    fetch: (request) => {
      requests.push(request);
      // fetch rejects with whatever it got (abort reasons may be any value), so the fake does too.
      // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors
      return Promise.reject(cause);
    },
  };
}

/** Builds a response whose body is `body` serialized as JSON. */
export function jsonResponse(
  body: unknown,
  status: number,
  contentType = "application/json",
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": contentType },
  });
}
