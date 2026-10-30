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
    ["a relative path", "/api"],
    ["blank text", "  "],
    ["a non-http scheme", "ftp://server.test"],
  ])("rejects %s as base URL with a clear TypeError", (_case, baseUrl) => {
    expect(() => createApiClient({ baseUrl })).toThrow(
      new TypeError(
        `Invalid API base URL "${baseUrl}": expected an absolute http or https URL such as "http://192.168.1.10:8460".`,
      ),
    );
  });

  it("keeps the parser error as cause of an unparsable base URL", () => {
    let caught: unknown;
    try {
      createApiClient({ baseUrl: "not a url" });
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(TypeError);
    expect((caught as TypeError).cause).toBeInstanceOf(TypeError);
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
