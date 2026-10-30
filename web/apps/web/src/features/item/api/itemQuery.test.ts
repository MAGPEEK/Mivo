import { ApiError, clientErrorCodes, createApiClient } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { isItemNotFound, isNetworkFailure, itemQueryKey, itemQueryOptions } from "./itemQuery";

function apiError(status: number, code: string): ApiError {
  return new ApiError({ status, code, title: "Problem" });
}

describe("isItemNotFound", () => {
  it.each([
    { name: "404 not_found", error: apiError(404, "not_found"), expected: true },
    { name: "400 for a malformed ID", error: apiError(400, "invalid"), expected: true },
    { name: "500", error: apiError(500, "internal"), expected: false },
    {
      name: "a network error",
      error: apiError(0, clientErrorCodes.networkError),
      expected: false,
    },
    { name: "a plain Error", error: new Error("boom"), expected: false },
  ])("is $expected for $name", ({ error, expected }) => {
    expect(isItemNotFound(error)).toBe(expected);
  });
});

describe("isNetworkFailure", () => {
  it.each([
    { name: "a network error", error: apiError(0, clientErrorCodes.networkError), expected: true },
    { name: "a 500 problem", error: apiError(500, "internal"), expected: false },
    {
      name: "an unexpected response",
      error: apiError(502, clientErrorCodes.unexpectedResponse),
      expected: false,
    },
    { name: "a plain TypeError", error: new TypeError("Failed to fetch"), expected: false },
  ])("is $expected for $name", ({ error, expected }) => {
    expect(isNetworkFailure(error)).toBe(expected);
  });
});

describe("itemQueryOptions", () => {
  const options = itemQueryOptions(createApiClient(), "item-1");
  const retry = options.retry as (failureCount: number, error: Error) => boolean;

  it("caches per item", () => {
    expect(options.queryKey).toEqual(itemQueryKey("item-1"));
    expect(itemQueryKey("item-1")).not.toEqual(itemQueryKey("item-2"));
  });

  it("retries other failures once", () => {
    const error = apiError(503, "unavailable");

    expect(retry(0, error)).toBe(true);
    expect(retry(1, error)).toBe(false);
  });

  it("never retries an unknown item", () => {
    expect(retry(0, apiError(404, "not_found"))).toBe(false);
  });
});
