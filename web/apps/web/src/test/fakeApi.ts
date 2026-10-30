import { createApiClient, type ApiClient, type Problem, type SystemInfo } from "@mivo/api-client";

/** Stands in for `fetch`: answers the requests of the API client without a network. */
export type FakeFetch = (request: Request) => Promise<Response>;

// Absolute, because Node's Request cannot resolve the client's default relative URLs.
const fakeServerUrl = "http://server.test";

/** System info the fake server reports unless a test passes its own fetch. */
export const fakeSystemInfo: SystemInfo = {
  productName: "Test Server",
  version: "1.2.3",
  serverId: "6f1c2a4e-8d3b-4c55-9a7e-0b2d4f6a8c10",
  apiVersion: "1",
};

/** Answers every request with `body` as JSON and the given status. */
export function respondWithJson(
  body: unknown,
  status: number,
  contentType = "application/json",
): FakeFetch {
  return () =>
    Promise.resolve(
      new Response(JSON.stringify(body), { status, headers: { "Content-Type": contentType } }),
    );
}

/** Answers every request with RFC 9457 problem details for `status`. */
export function respondWithProblem(problem: Problem): FakeFetch {
  return respondWithJson(problem, problem.status, "application/problem+json");
}

/** Never answers, so the request stays loading. */
export function respondNever(): FakeFetch {
  return () => new Promise<Response>(() => undefined);
}

/** A healthy server: system info answers with {@link fakeSystemInfo}. */
export const healthyServer: FakeFetch = respondWithJson(fakeSystemInfo, 200);

/** Creates a real API client whose requests go to `fetch` instead of the network. */
export function createFakeApiClient(fetch: FakeFetch = healthyServer): ApiClient {
  return createApiClient({ baseUrl: fakeServerUrl, fetch });
}
