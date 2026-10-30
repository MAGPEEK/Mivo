import { afterEach, describe, expect, expectTypeOf, it, vi } from "vitest";

import { createApiClient } from "./client";
import type { HealthResponse, Problem, SystemInfo } from "./schemas";
import { jsonResponse, respondWith, testBaseUrl } from "./test/fakeFetch";

const systemInfo: SystemInfo = {
  productName: "Media Server",
  version: "0.1.0",
  serverId: "6f1c2a4e-8d3b-4c55-9a7e-0b2d4f6a8c10",
  apiVersion: "1",
};

const unavailableHealth: HealthResponse = {
  status: "unavailable",
  checks: [{ name: "database", status: "unavailable", detail: "database unreachable" }],
};

const expectedHttpUrl =
  'Invalid API base URL: expected an http(s) URL such as "http://192.168.1.10:8460"';
// Far beyond any real server URL; the messages must stay short anyway.
const hugeLength = 10_000;

function captureCreateError(baseUrl: string): unknown {
  try {
    createApiClient({ baseUrl });
  } catch (error) {
    return error;
  }
  return undefined;
}

describe("createApiClient", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns typed data for GET /api/v1/system/info", async () => {
    const fake = respondWith(() => jsonResponse(systemInfo, 200));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { data, error } = await client.GET("/api/v1/system/info");

    expect(data).toEqual(systemInfo);
    expect(error).toBeUndefined();
    expectTypeOf(data).toEqualTypeOf<SystemInfo | undefined>();
    expect(fake.requests).toHaveLength(1);
    expect(fake.requests[0]?.method).toBe("GET");
    expect(fake.requests[0]?.url).toBe(`${testBaseUrl}/api/v1/system/info`);
  });

  it("returns the typed health body when /api/v1/health answers 503", async () => {
    const fake = respondWith(() => jsonResponse(unavailableHealth, 503));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { data, error, response } = await client.GET("/api/v1/health");

    expect(response.status).toBe(503);
    expect(data).toBeUndefined();
    expect(error).toEqual(unavailableHealth);
    // 503 carries a HealthResponse; every other failure status carries problem details.
    expectTypeOf(error).toEqualTypeOf<HealthResponse | Problem | undefined>();
  });

  it("requests relative URLs by default so the page origin applies", async () => {
    // Node has no page origin, so this Request stand-in resolves relative URLs the way a
    // browser does against the document. createClient reads globalThis.Request on creation.
    const pageOrigin = "http://page.test";
    class PageRequest extends Request {
      constructor(input: RequestInfo | URL, init?: RequestInit) {
        super(typeof input === "string" ? new URL(input, pageOrigin) : input, init);
      }
    }
    vi.stubGlobal("Request", PageRequest);
    const fake = respondWith(() => jsonResponse(systemInfo, 200));
    const client = createApiClient({ fetch: fake.fetch });

    await client.GET("/api/v1/system/info");

    expect(fake.requests[0]?.url).toBe(`${pageOrigin}/api/v1/system/info`);
  });

  it.each([
    ["an origin", "http://192.168.1.10:8460"],
    ["an https origin with a path prefix", "https://media.example.test/server/"],
  ])("accepts %s as base URL", (_case, baseUrl) => {
    expect(() => createApiClient({ baseUrl })).not.toThrow();
  });

  it.each([
    ["a host without scheme", "192.168.1.10:8460"],
    ["a host name read as scheme", "localhost:8460"],
    ["a host name with a query read as scheme", "media.local:8460/?u=a@b&token=SECRET"],
    ["a scheme without slashes", "javascript:alert(1)"],
    ["a data URL", "data:text/plain,SECRET"],
    ["a relative path", "/api"],
    ["blank text", "  "],
    ["unparsable text with a fragment", "not a url#api"],
    ["an unparsable URL with credentials", "http://admin:secret@"],
    ["a scheme-less URL with credentials", "admin:secret@server.test:8460"],
    ["a password with a slash and a hash", "http://admin:se/c#ret@host:x"],
    ["credentials with a full-width at sign", "http://admin:secret＠server.test"],
    ["a scheme-less host with a full-width at sign", "admin＠server.test:8460?token=SECRET"],
    ["a huge text", "a".repeat(hugeLength)],
    ["a huge scheme", `${"x".repeat(hugeLength)}://server.test`],
    // The parser strips leading spaces and reads one slash or a backslash as "//" for
    // special schemes, so these parse with the protocol "ftp:", but the input does not
    // literally start with "ftp://".
    ["a non-http scheme after leading spaces", "  ftp://server.test"],
    ["a non-http scheme after a tab", "\tftp://server.test"],
    ["a non-http scheme with one slash", "ftp:/server.test"],
    ["a non-http scheme with backslashes", "ftp:\\\\server.test"],
    ["an upper-case non-http scheme with one slash", "FTP:/server.test"],
  ])("rejects %s as base URL without repeating any of it", (_case, baseUrl) => {
    expect(() => createApiClient({ baseUrl })).toThrow(new TypeError(`${expectedHttpUrl}.`));
  });

  it.each([
    ["a non-http scheme", "ftp://server.test", "ftp"],
    ["an upper-case scheme", "FTP://server.test", "ftp"],
    ["a mixed-case scheme", "FtP://server.test", "ftp"],
    ["a non-http scheme with a query", "ftp://server.test/?email=me@x.y&token=SECRET", "ftp"],
    ["a non-http scheme with credentials", "ftp://admin:secret@server.test", "ftp"],
    ["a non-http scheme with a huge host", `ftp://${"a".repeat(hugeLength)}`, "ftp"],
  ])("rejects %s as base URL and repeats only the scheme", (_case, baseUrl, scheme) => {
    expect(() => createApiClient({ baseUrl })).toThrow(
      new TypeError(`${expectedHttpUrl}, got the scheme "${scheme}".`),
    );
  });

  it.each([
    [19, true],
    [20, true],
    [21, false],
  ])("shows a scheme of %i characters: %s", (length, shown) => {
    const scheme = "s".repeat(length);
    const received = shown ? `, got the scheme "${scheme}"` : "";

    expect(() => createApiClient({ baseUrl: `${scheme}://server.test` })).toThrow(
      new TypeError(`${expectedHttpUrl}${received}.`),
    );
  });

  it.each([
    ["a user name", "http://admin@server.test:8460", "credentials are"],
    ["a password", "http://admin:secret@server.test/", "credentials are"],
    ["a query", "http://server.test/api?token=abc", "a query is"],
    ["an empty query", "http://server.test/?", "a query is"],
    ["a query and a fragment", "http://server.test/?a#b", "a query is"],
    ["a fragment", "http://server.test/#api", "a fragment is"],
    ["a question mark in the fragment", "http://server.test/#a?b", "a fragment is"],
    ["an empty fragment", "http://server.test/#", "a fragment is"],
  ])("rejects a base URL with %s and names only that part", (_case, baseUrl, rejectedPart) => {
    expect(() => createApiClient({ baseUrl })).toThrow(
      new TypeError(`Invalid API base URL: ${rejectedPart} not allowed.`),
    );
  });

  it.each([
    ["an http URL", "https://admin:hunter2@server.test"],
    ["an ftp URL", "ftp://admin:hunter2@server.test"],
    ["an unparsable URL", "http://admin:hunter2@"],
    ["a scheme-less URL", "admin:hunter2@server.test:8460"],
    ["a scheme-less URL without user name", ":hunter2@server.test"],
    ["a URL with an @ in the password", "ftp://admin:hun@ter2@server.test"],
    ["a URL with a full-width at sign", "ftp://admin:hunter2＠server.test"],
  ])("never repeats the credentials of %s", (_case, baseUrl) => {
    const caught = captureCreateError(baseUrl);

    expect(caught).toBeInstanceOf(TypeError);
    const { message } = caught as TypeError;
    expect(message).not.toMatch(/admin|hun|ter2/);
  });

  it.each([
    ["an ftp URL", "ftp://server.test/?token=SECRET#key"],
    ["an http URL with a full-width at sign", "http://server.test/a＠b?token=SECRET#key"],
  ])("never repeats the query or fragment of %s", (_case, baseUrl) => {
    const caught = captureCreateError(baseUrl);

    expect(caught).toBeInstanceOf(TypeError);
    expect((caught as TypeError).message).not.toMatch(/token|SECRET|key/);
  });

  // The parser reads a password that starts with digits and contains "#", "?" or "/" as a
  // port, and the rest as fragment, query or path, so the "host" is the user name.
  it.each([
    ["an unparsable URL", "http://admin:SECRET@"],
    ["a non-http URL", "ftp://server.test/?token=SECRET"],
    ["an http URL with a query", "http://server.test/?token=SECRET"],
    ["a password read as port and fragment", "http://admin:4711#pw@server.test"],
    ["a password read as port and query", "http://admin:4711?pw@server.test"],
    ["a password read as path", "http://admin:/pw@server.test?x"],
    ["a password read as port, full-width at sign", "http://admin:4711#pw＠server.test"],
    ["a password read as path, full-width at sign", "http://admin:/pw＠server.test?x"],
    ["a password read as port, encoded at sign", "http://admin:4711#pw%40server.test"],
    ["a password read as path, encoded at sign", "http://admin:/pw%40server.test?x"],
    ["a token in the path", "http://server.test/api;token=SECRET?x"],
  ])("attaches nothing of %s to the error", (_case, baseUrl) => {
    const caught = captureCreateError(baseUrl);

    expect(caught).toBeInstanceOf(TypeError);
    const error = caught as TypeError;
    // Node's own "Invalid URL" error carries the raw input in `input`; it must not travel along.
    expect(error.cause).toBeUndefined();
    const carried = Object.getOwnPropertyNames(error)
      .map((name) => String(Reflect.get(error, name)))
      .join("\n");
    expect(carried).not.toMatch(/admin|4711|pw|SECRET|token/);
    expect(carried).not.toContain(baseUrl);
  });

  it("uses globalThis.fetch when no fetch is given", async () => {
    const fake = respondWith(() => jsonResponse(systemInfo, 200));
    vi.stubGlobal("fetch", fake.fetch);
    const client = createApiClient({ baseUrl: testBaseUrl });

    const { data } = await client.GET("/api/v1/system/info");

    expect(data).toEqual(systemInfo);
    expect(fake.requests).toHaveLength(1);
  });
});
