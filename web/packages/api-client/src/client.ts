import createClient, { type Client } from "openapi-fetch";

import type { paths } from "./generated/schema";

/** HTTP client whose methods, paths and payloads are typed by `api/openapi.yaml`. */
export type ApiClient = Client<paths>;

/** Settings for {@link createApiClient}. */
export interface ApiClientOptions {
  /**
   * Origin (and optional path prefix) the `/api/v1/...` paths are appended to. Defaults to
   * same origin, which suits the web app served by the server itself; the TV app passes the
   * server URL it discovered.
   */
  readonly baseUrl?: string;
  /** Replaces `globalThis.fetch`, for example with a fake in tests. */
  readonly fetch?: (request: Request) => Promise<Response>;
}

// An empty base URL keeps request URLs relative, so the browser resolves them against the
// page origin. The client never reads window.location itself, because the TV app runs on a
// different origin than the server.
const sameOriginBaseUrl = "";

/** Creates a typed client for the server API. */
export function createApiClient(options: ApiClientOptions = {}): ApiClient {
  const { baseUrl = sameOriginBaseUrl, fetch } = options;
  return createClient<paths>(fetch === undefined ? { baseUrl } : { baseUrl, fetch });
}
