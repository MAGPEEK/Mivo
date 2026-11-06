import { describe, expect, expectTypeOf, it } from "vitest";

import { ApiError, clientErrorCodes, noResponseStatus } from "./apiError";
import { createApiClient } from "./client";
import { requestData } from "./requestData";
import type { Problem, SystemInfo } from "./schemas";
import {
  jsonResponse,
  rejectWith,
  respondWith,
  testBaseUrl,
  type FakeFetch,
} from "./test/fakeFetch";

const problemMediaType = "application/problem+json";

const systemInfo: SystemInfo = {
  productName: "Media Server",
  version: "0.1.0",
  serverId: "6f1c2a4e-8d3b-4c55-9a7e-0b2d4f6a8c10",
  apiVersion: "1",
  libraryKinds: ["movies", "shows"],
};

const notFound: Problem = {
  type: "about:blank",
  title: "Not Found",
  status: 404,
  detail: "No route matches the request.",
  code: "not_found",
};

function requestSystemInfo(fake: FakeFetch): Promise<SystemInfo> {
  const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });
  return requestData(client.GET("/api/v1/system/info"));
}

async function captureError(fake: FakeFetch): Promise<unknown> {
  try {
    await requestSystemInfo(fake);
  } catch (error) {
    return error;
  }
  throw new Error("expected the request to fail");
}

async function captureApiError(fake: FakeFetch): Promise<ApiError> {
  const error = await captureError(fake);
  expect(error).toBeInstanceOf(ApiError);
  return error as ApiError;
}

describe("requestData", () => {
  it("returns the typed data of a successful call", async () => {
    const data = await requestSystemInfo(respondWith(() => jsonResponse(systemInfo, 200)));

    expect(data).toEqual(systemInfo);
    expectTypeOf(data).toEqualTypeOf<SystemInfo>();
  });

  it("turns a problem+json 404 into an ApiError with its code", async () => {
    const error = await captureApiError(
      respondWith(() => jsonResponse(notFound, 404, problemMediaType)),
    );

    expect(error.code).toBe("not_found");
    expect(error.status).toBe(404);
    expect(error.title).toBe("Not Found");
    expect(error.detail).toBe("No route matches the request.");
    expect(error.message).toBe("Not Found");
    expect(error.name).toBe("ApiError");
  });

  it("recognises problem+json with parameters and any letter case", async () => {
    const error = await captureApiError(
      respondWith(() => jsonResponse(notFound, 404, "Application/Problem+JSON; charset=utf-8")),
    );

    expect(error.code).toBe("not_found");
  });

  it("uses the HTTP status over the problem's advisory status member", async () => {
    const error = await captureApiError(
      respondWith(() => jsonResponse({ ...notFound, status: 400 }, 404, problemMediaType)),
    );

    expect(error.status).toBe(404);
  });

  it("keeps detail undefined when the problem has none", async () => {
    const withoutDetail: Problem = {
      type: notFound.type,
      title: notFound.title,
      status: notFound.status,
      code: notFound.code,
    };
    const error = await captureApiError(
      respondWith(() => jsonResponse(withoutDetail, 404, problemMediaType)),
    );

    expect(error.code).toBe("not_found");
    expect(error.detail).toBeUndefined();
  });

  it("carries field and libraryId of a library problem", async () => {
    const overlap = {
      ...notFound,
      code: "folder_overlap",
      field: "folders[1]",
      libraryId: "lib-1",
    };
    const error = await captureApiError(
      respondWith(() => jsonResponse(overlap, 409, problemMediaType)),
    );

    expect(error.code).toBe("folder_overlap");
    expect(error.field).toBe("folders[1]");
    expect(error.libraryId).toBe("lib-1");
  });

  it("leaves field and libraryId undefined when the problem has none", async () => {
    const error = await captureApiError(
      respondWith(() => jsonResponse(notFound, 404, problemMediaType)),
    );

    expect(error.field).toBeUndefined();
    expect(error.libraryId).toBeUndefined();
  });

  it.each([
    { name: "the body member", body: { retryAfterSeconds: 42 }, header: "7", expected: 42 },
    { name: "the header without a member", body: {}, header: " 7 ", expected: 7 },
    {
      name: "the header for a broken member",
      body: { retryAfterSeconds: -3 },
      header: "9",
      expected: 9,
    },
    {
      name: "nothing for a broken header",
      body: { retryAfterSeconds: "x" },
      header: "soon",
      expected: undefined,
    },
    { name: "nothing for a zero header", body: {}, header: "0", expected: undefined },
  ])("takes retryAfterSeconds from $name", async ({ body, header, expected }) => {
    const throttled = { ...notFound, status: 429, code: "too_many_attempts", ...body };
    const error = await captureApiError(
      respondWith(() => {
        const response = jsonResponse(throttled, 429, problemMediaType);
        response.headers.set("Retry-After", header);
        return response;
      }),
    );

    expect(error.code).toBe("too_many_attempts");
    expect(error.retryAfterSeconds).toBe(expected);
  });

  it("reports unexpected_response for a non-string field", async () => {
    const error = await captureApiError(
      respondWith(() => jsonResponse({ ...notFound, field: 3 }, 400, problemMediaType)),
    );

    expect(error.code).toBe("unexpected_response");
  });

  it.each([
    ["a JSON body that is no problem", () => jsonResponse({ message: "boom" }, 500)],
    ["a problem shape without the problem media type", () => jsonResponse(notFound, 500)],
    [
      "problem+json without a code",
      () => jsonResponse({ title: "Oops", status: 500 }, 500, problemMediaType),
    ],
    [
      "problem+json with a non-string detail",
      () => jsonResponse({ ...notFound, detail: 42 }, 500, problemMediaType),
    ],
    ["problem+json that is not an object", () => jsonResponse("boom", 500, problemMediaType)],
    ["problem+json that is null", () => jsonResponse(null, 500, problemMediaType)],
    ["a plain-text body", () => new Response("Bad Gateway", { status: 500 })],
    [
      "an empty body",
      () => new Response(null, { status: 500, headers: { "Content-Length": "0" } }),
    ],
  ])("reports unexpected_response for %s", async (_case, createResponse) => {
    const error = await captureApiError(respondWith(createResponse));

    expect(error.code).toBe(clientErrorCodes.unexpectedResponse);
    expect(error.status).toBe(500);
    expect(error.detail).toBe("HTTP status 500 without problem details.");
  });

  it("turns a rejected fetch into an ApiError network_error", async () => {
    const cause = new TypeError("Failed to fetch");

    const error = await captureApiError(rejectWith(cause));

    expect(error.code).toBe(clientErrorCodes.networkError);
    expect(error.status).toBe(noResponseStatus);
    expect(error.detail).toBe("Failed to fetch");
    expect(error.cause).toBe(cause);
  });

  it("reports network_error without detail when fetch rejects with a non-error", async () => {
    const error = await captureApiError(rejectWith("offline"));

    expect(error.code).toBe(clientErrorCodes.networkError);
    expect(error.detail).toBeUndefined();
  });

  it("rethrows an abort unchanged so cancellation stays recognisable", async () => {
    const abort = new DOMException("The operation was aborted.", "AbortError");

    const error = await captureError(rejectWith(abort));

    expect(error).toBe(abort);
  });

  it.each([
    [
      "Content-Length 0",
      () => new Response(null, { status: 200, headers: { "Content-Length": "0" } }),
    ],
    ["an empty body without Content-Length", () => new Response("", { status: 200 })],
  ])("reports unexpected_response with the status for a 2xx with %s", async (_case, create) => {
    const error = await captureApiError(respondWith(create));

    expect(error.code).toBe(clientErrorCodes.unexpectedResponse);
    expect(error.status).toBe(200);
    expect(error.detail).toBe("HTTP status 200 without a body.");
  });

  it("resolves to undefined for 204 No Content", async () => {
    const noContent = Promise.resolve({
      data: undefined,
      response: new Response(null, { status: 204 }),
    });

    await expect(requestData(noContent)).resolves.toBeUndefined();
  });

  it("reports unexpected_response when a 2xx body is not valid JSON", async () => {
    const error = await captureApiError(
      respondWith(
        () => new Response("<html>", { status: 200, headers: { "Content-Type": "text/html" } }),
      ),
    );

    expect(error.code).toBe(clientErrorCodes.unexpectedResponse);
    expect(error.status).toBe(noResponseStatus);
    expect(error.cause).toBeInstanceOf(SyntaxError);
  });
});
