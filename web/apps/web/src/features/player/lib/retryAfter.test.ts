import { describe, expect, it } from "vitest";

import { parseRetryAfter, retryAfterFromNetworkDetails } from "./retryAfter";

describe("parseRetryAfter", () => {
  it.each([
    { value: "2", expected: 2 },
    { value: " 30 ", expected: 30 },
    { value: "0", expected: undefined },
    { value: "-1", expected: undefined },
    { value: "1.5", expected: undefined },
    { value: "Wed, 21 Oct 2026 07:28:00 GMT", expected: undefined },
    { value: "300", expected: 300 },
    { value: "301", expected: 300 },
    { value: "99999999999999999999", expected: 300 },
    { value: "", expected: undefined },
    { value: null, expected: undefined },
    { value: undefined, expected: undefined },
  ])("reads $value as $expected", ({ value, expected }) => {
    expect(parseRetryAfter(value)).toBe(expected);
  });
});

describe("retryAfterFromNetworkDetails", () => {
  it("reads the header of an XMLHttpRequest", () => {
    const request = {
      getResponseHeader: (name: string) => (name === "Retry-After" ? "4" : null),
    };

    expect(retryAfterFromNetworkDetails(request)).toBe(4);
  });

  it("reads the header of a fetch Response", () => {
    const response = new Response(null, { status: 503, headers: { "Retry-After": "6" } });

    expect(retryAfterFromNetworkDetails(response)).toBe(6);
  });

  it.each([
    { name: "no details", details: null },
    { name: "a string", details: "503" },
    { name: "an object without headers", details: {} },
    { name: "headers without lookup", details: { headers: {} } },
    { name: "null headers", details: { headers: null } },
  ])("returns undefined for $name", ({ details }) => {
    expect(retryAfterFromNetworkDetails(details)).toBeUndefined();
  });
});
