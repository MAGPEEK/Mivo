import createClient, { type Client } from "openapi-fetch";

import type { paths } from "./generated/schema";

/** HTTP client whose methods, paths and payloads are typed by `api/openapi.yaml`. */
export type ApiClient = Client<paths>;

/** Settings for {@link createApiClient}. */
export interface ApiClientOptions {
  /**
   * Origin (and optional path prefix) the `/api/v1/...` paths are appended to. Defaults to
   * same origin, which suits the web app served by the server itself; the TV app passes the
   * server URL it discovered. Anything but the empty default must be an absolute http(s) URL.
   */
  readonly baseUrl?: string;
  /** Replaces `globalThis.fetch`, for example with a fake in tests. */
  readonly fetch?: (request: Request) => Promise<Response>;
}

// An empty base URL keeps request URLs relative, so the browser resolves them against the
// page origin. The client never reads window.location itself, because the TV app runs on a
// different origin than the server.
const sameOriginBaseUrl = "";

const supportedProtocols: readonly string[] = ["http:", "https:"];

/**
 * Creates a typed client for the server API.
 * @throws TypeError when `baseUrl` is neither empty nor an absolute http(s) URL. A server URL
 *   the user typed fails here with a clear message instead of as a `network_error` later.
 */
export function createApiClient(options: ApiClientOptions = {}): ApiClient {
  const { baseUrl = sameOriginBaseUrl, fetch } = options;
  if (baseUrl !== sameOriginBaseUrl) {
    assertServerUrl(baseUrl);
  }
  return createClient<paths>(fetch === undefined ? { baseUrl } : { baseUrl, fetch });
}

function assertServerUrl(baseUrl: string): void {
  // URL.canParse is missing on webOS 6 (Chromium 79); the constructor works everywhere.
  let url: URL;
  try {
    url = new URL(baseUrl);
  } catch (cause) {
    throw new TypeError(invalidBaseUrlMessage(baseUrl), { cause });
  }
  // "localhost:8460" parses with the scheme "localhost:", so the protocol needs its own check.
  if (!supportedProtocols.includes(url.protocol)) {
    throw new TypeError(invalidBaseUrlMessage(baseUrl));
  }
}

function invalidBaseUrlMessage(baseUrl: string): string {
  return `Invalid API base URL "${baseUrl}": expected an absolute http or https URL such as "http://192.168.1.10:8460".`;
}
