import createClient, { type Client } from "openapi-fetch";

import type { paths } from "./generated/schema";

/** HTTP client whose methods, paths and payloads are typed by `api/openapi.yaml`. */
export type ApiClient = Client<paths>;

/** Settings for {@link createApiClient}. */
export interface ApiClientOptions {
  /**
   * Origin (and optional path prefix) the `/api/v1/...` paths are appended to. Defaults to
   * same origin, which suits the web app served by the server itself; the TV app passes the
   * server URL it discovered. Anything but the empty default must be an absolute http(s) URL
   * without credentials, query or fragment.
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

// Stands in for a user name and password in error messages.
const redactedUserinfo = "***";
// A leading "scheme://" survives the redaction, so the message still shows the scheme.
const schemeWithSlashes = /^[a-z][a-z\d+.-]*:\/\//i;

/**
 * Creates a typed client for the server API.
 * @throws TypeError when `baseUrl` is neither empty nor an absolute http(s) URL without
 *   credentials, query or fragment. A server URL the user typed fails here with a clear
 *   message instead of as a `network_error` later. The message never contains credentials,
 *   a query or a fragment of the input.
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
    // `cause` is for diagnostics only: webOS 6 ignores the options argument of Error
    // constructors, so nothing may depend on it.
    throw new TypeError(invalidBaseUrlMessage(baseUrl), { cause });
  }
  // "localhost:8460" parses with the scheme "localhost:", so the protocol needs its own check.
  if (!supportedProtocols.includes(url.protocol)) {
    throw new TypeError(invalidBaseUrlMessage(baseUrl));
  }
  const rejectedPart = disallowedPart(url);
  if (rejectedPart !== undefined) {
    // origin + pathname never contains credentials, a query or a fragment.
    throw new TypeError(
      `Invalid API base URL "${url.origin}${url.pathname}": ${rejectedPart} not allowed.`,
    );
  }
}

// Request paths are appended to the base URL as text, so a query or fragment would swallow
// them. Credentials would travel with every request; the API authenticates differently.
function disallowedPart(url: URL): string | undefined {
  if (url.username !== "" || url.password !== "") {
    return "credentials are";
  }
  // `search` and `hash` are empty for a bare "?" or "#", but the serialized URL keeps them.
  // The first "#" in `href` always starts the fragment, and a "?" before it the query:
  // elsewhere both characters are percent-encoded or belong to the fragment.
  const fragmentStart = url.href.indexOf("#");
  const beforeFragment = fragmentStart === -1 ? url.href : url.href.slice(0, fragmentStart);
  if (beforeFragment.includes("?")) {
    return "a query is";
  }
  return fragmentStart === -1 ? undefined : "a fragment is";
}

function invalidBaseUrlMessage(baseUrl: string): string {
  return `Invalid API base URL "${redactForMessage(baseUrl)}": expected an absolute http or https URL such as "http://192.168.1.10:8460".`;
}

/**
 * The input as it may appear in a message that ends up in logs or on screen: without user
 * name, password, query or fragment.
 *
 * Works on the text, because the input may not parse at all, and a scheme-less
 * "admin:secret@host" parses with the user name as scheme. Everything before the last "@"
 * is treated as userinfo, so an "@" in a path or query only hides more than needed.
 */
function redactForMessage(baseUrl: string): string {
  const userinfoEnd = baseUrl.lastIndexOf("@");
  let redacted = baseUrl;
  if (userinfoEnd !== -1) {
    // A scheme contains no "@", so the last "@" always follows it.
    const scheme = schemeWithSlashes.exec(baseUrl)?.[0] ?? "";
    redacted = `${scheme}${redactedUserinfo}${baseUrl.slice(userinfoEnd)}`;
  }
  const queryOrFragmentStart = redacted.search(/[?#]/);
  return queryOrFragmentStart === -1 ? redacted : redacted.slice(0, queryOrFragmentStart);
}
