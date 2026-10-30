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
const exampleServerUrl = "http://192.168.1.10:8460";

// The messages end up in logs and on screen, and the input may be anything the user pasted,
// tokens included. They therefore never repeat host, port, path, query, fragment or
// credentials: the parser reads a password like "4711#pw" as a port and moves the rest into
// the fragment, so no parsed part except the scheme is known to be free of secrets. The
// @throws doc of createApiClient spells the limit out; keep it in sync.

// Longer than the schemes people actually type; a longer "scheme" is more likely pasted text.
const maxShownSchemeLength = 20;
const schemeSeparator = ":";

/**
 * Creates a typed client for the server API.
 * @throws TypeError when `baseUrl` is neither empty nor an absolute http(s) URL without
 *   credentials, query or fragment. A server URL the user typed fails here with a clear
 *   message instead of as a `network_error` later. The message names the problem
 *   ("credentials are not allowed", "a query is not allowed", "a fragment is not allowed" or
 *   "expected an http(s) URL") and repeats nothing of the input except, for a URL with a
 *   scheme other than http or https, that scheme. The scheme is shown only when the input
 *   starts with `scheme://` and the scheme has at most 20 characters. Nothing is shown when
 *   - the input does not parse as a URL,
 *   - the scheme is not followed by "//", e.g. `media.local:8460`, `localhost:8460`,
 *     `javascript:…`, `data:…` or `admin:secret@host`,
 *   - the scheme is longer than 20 characters,
 *   - the input is an http(s) URL with credentials, a query or a fragment.
 *
 *   The error has no `cause`.
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
  } catch {
    // The parser error is deliberately not kept as cause: Node's carries the raw input in
    // `input`, and Firefox repeats it in the message.
    throw new TypeError(expectedHttpUrlMessage(""));
  }
  // "localhost:8460" parses with the scheme "localhost:", so the protocol needs its own check.
  if (!supportedProtocols.includes(url.protocol)) {
    throw new TypeError(expectedHttpUrlMessage(shownScheme(baseUrl, url.protocol)));
  }
  const rejectedPart = disallowedPart(url);
  if (rejectedPart !== undefined) {
    throw new TypeError(`Invalid API base URL: ${rejectedPart} not allowed.`);
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

/** The message for input that is no http(s) URL; `scheme` is "" when nothing may be shown. */
function expectedHttpUrlMessage(scheme: string): string {
  const received = scheme === "" ? "" : `, got the scheme "${scheme}"`;
  return `Invalid API base URL: expected an http(s) URL such as "${exampleServerUrl}"${received}.`;
}

/**
 * The scheme of `protocol` (which ends with ":") when it is safe to show, otherwise "". Only
 * a scheme followed by "//" counts: without the slashes the parser also reads a host name
 * ("media.local:8460") or a user name ("admin:secret@host") as the scheme.
 */
function shownScheme(baseUrl: string, protocol: string): string {
  const startsWithScheme = baseUrl.toLowerCase().startsWith(`${protocol}//`);
  const scheme = protocol.slice(0, -schemeSeparator.length);
  return startsWithScheme && scheme.length <= maxShownSchemeLength ? scheme : "";
}
