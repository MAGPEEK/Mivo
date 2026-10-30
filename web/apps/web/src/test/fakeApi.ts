import {
  createApiClient,
  type ApiClient,
  type LibraryList,
  type Problem,
  type SystemInfo,
} from "@mivo/api-client";

/** Stands in for `fetch`: answers the requests of the API client without a network. */
export type FakeFetch = (request: Request) => Promise<Response>;

// Absolute, because Node's Request cannot resolve the client's default relative URLs.
const fakeServerUrl = "http://server.test";

const notFoundStatus = 404;

/** System info the fake server reports unless a test passes its own fetch. */
export const fakeSystemInfo: SystemInfo = {
  productName: "Test Server",
  version: "1.2.3",
  serverId: "6f1c2a4e-8d3b-4c55-9a7e-0b2d4f6a8c10",
  apiVersion: "1",
};

/** A JSON response with the given status. */
export function jsonResponse(
  body: unknown,
  status = 200,
  contentType = "application/json",
): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": contentType } });
}

/** RFC 9457 problem details with the given status and code, as the server sends them. */
export function problemResponse(status: number, code: string): Response {
  const problem: Problem = { type: "about:blank", title: code, status, code };
  return jsonResponse(problem, status, "application/problem+json");
}

/** Answers every request with `body` as JSON and the given status. */
export function respondWithJson(
  body: unknown,
  status: number,
  contentType = "application/json",
): FakeFetch {
  return () => Promise.resolve(jsonResponse(body, status, contentType));
}

/** Answers every request with RFC 9457 problem details for `status`. */
export function respondWithProblem(problem: Problem): FakeFetch {
  return respondWithJson(problem, problem.status, "application/problem+json");
}

/** Never answers, so the request stays loading. */
export function respondNever(): FakeFetch {
  return () => new Promise<Response>(() => undefined);
}

/** A request that matched a {@link FakeRoute}, with the values of its `:name` path segments. */
export interface FakeRouteRequest {
  readonly request: Request;
  readonly url: URL;
  readonly params: Readonly<Record<string, string>>;
}

/** One endpoint of a {@link FakeServer}. */
export interface FakeRoute {
  readonly method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  /** API path; segments such as `:libraryId` match any single segment. */
  readonly path: string;
  readonly respond: (request: FakeRouteRequest) => Response | Promise<Response>;
}

/** An in-memory server that answers the API client by method and path. */
export interface FakeServer {
  /** Pass it to {@link createFakeApiClient} or `renderApp`. */
  readonly fetch: FakeFetch;
  /** Every request the server received, oldest first. */
  readonly requests: readonly Request[];
  /** How many requests matched `method` and `path` (the path as sent, without query). */
  readonly countRequests: (method: string, path: string) => number;
}

/** The routes every fake server has unless a test overrides them. */
export const defaultRoutes: readonly FakeRoute[] = [
  {
    method: "GET",
    path: "/api/v1/system/info",
    respond: () => jsonResponse(fakeSystemInfo),
  },
  {
    method: "GET",
    path: "/api/v1/libraries",
    respond: () => jsonResponse({ libraries: [] } satisfies LibraryList),
  },
];

function matchPath(pattern: string, path: string): Record<string, string> | undefined {
  const patternSegments = pattern.split("/");
  const pathSegments = path.split("/");
  if (patternSegments.length !== pathSegments.length) {
    return undefined;
  }
  const params: Record<string, string> = {};
  for (const [index, segment] of patternSegments.entries()) {
    const actual = pathSegments[index] ?? "";
    if (segment.startsWith(":")) {
      params[segment.slice(1)] = decodeURIComponent(actual);
    } else if (segment !== actual) {
      return undefined;
    }
  }
  return params;
}

/**
 * Creates a fake server. `routes` come first, so a test overrides a default route by
 * declaring the same method and path. Unknown endpoints answer 404 `not_found`, like the
 * real server for an unknown ID.
 */
export function createFakeServer(routes: readonly FakeRoute[] = []): FakeServer {
  const allRoutes = [...routes, ...defaultRoutes];
  const requests: Request[] = [];
  const fetch: FakeFetch = async (request) => {
    requests.push(request);
    const url = new URL(request.url);
    for (const route of allRoutes) {
      const params =
        route.method === request.method ? matchPath(route.path, url.pathname) : undefined;
      if (params !== undefined) {
        return route.respond({ request, url, params });
      }
    }
    return problemResponse(notFoundStatus, "not_found");
  };
  const countRequests = (method: string, path: string) =>
    requests.filter(
      (request) => request.method === method && new URL(request.url).pathname === path,
    ).length;
  return { fetch, requests, countRequests };
}

/** A healthy server: system info answers with {@link fakeSystemInfo}, no libraries exist. */
export const healthyServer: FakeFetch = (request) => createFakeServer().fetch(request);

/** Creates a real API client whose requests go to `fetch` instead of the network. */
export function createFakeApiClient(fetch: FakeFetch = healthyServer): ApiClient {
  return createApiClient({ baseUrl: fakeServerUrl, fetch });
}
