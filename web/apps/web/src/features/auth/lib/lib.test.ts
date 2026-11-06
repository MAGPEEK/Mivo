import { ApiError, type Me } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeMe } from "../../../test/fakeApi";
import { authHref, validateAuthSearch } from "./authSearch";
import { generalFailure, throttleSeconds } from "./authErrors";
import { formatCountdown, secondsUntil } from "./countdown";
import { checkName, checkNewPassword, checkRepeat } from "./credentials";
import { guardRedirect, type GuardLocation } from "./guard";
import { nextPathFor, sanitizeNextPath } from "./nextPath";
import { dockerRecoveryCommand, recoveryCommand } from "./recoveryCommand";
import { deriveSession, type QuerySnapshot, type SessionState } from "./sessionStatus";
import { mayAffectSession, unauthorizedAction } from "./unauthorized";

describe("sanitizeNextPath", () => {
  it.each([
    ["/items/1", "/items/1"],
    ["/libraries/lib-1?sort=title#top", "/libraries/lib-1?sort=title#top"],
    ["/a/../items/1", "/items/1"],
    ["/", "/"],
  ])("accepts the same-app path %s", (value, expected) => {
    expect(sanitizeNextPath(value)).toBe(expected);
  });

  it.each([
    ["a protocol-relative URL", "//evil.example/steal"],
    ["a backslash that browsers read as a slash", "/\\evil.example"],
    ["a backslash anywhere", "/items\\..\\..\\evil"],
    ["an absolute https URL", "https://evil.example/"],
    ["an absolute http URL", "http://evil.example"],
    ["a javascript URL", "javascript:alert(1)"],
    ["a relative path", "items/1"],
    ["a tab that browsers strip", "/\t/evil.example"],
    ["a newline", "/\n/evil.example"],
    ["a leading space", " /items/1"],
    ["an empty string", ""],
    ["the sign-in page", "/sign-in?next=/items/1"],
    ["the setup page", "/setup"],
    ["the profile picker", "/profiles"],
    ["an overlong value", `/${"a".repeat(2048)}`],
  ])("refuses %s", (_name, value) => {
    expect(sanitizeNextPath(value)).toBeUndefined();
  });

  it.each([undefined, null, 42, { path: "/items/1" }])("refuses the non-string %s", (value) => {
    expect(sanitizeNextPath(value)).toBeUndefined();
  });

  it("names no return target for Home", () => {
    expect(nextPathFor("/")).toBeUndefined();
    expect(nextPathFor("/items/1?t=4")).toBe("/items/1?t=4");
  });
});

describe("auth search", () => {
  it("keeps a safe next and a known reason", () => {
    expect(validateAuthSearch({ next: "/items/1", reason: "expired" })).toEqual({
      next: "/items/1",
      reason: "expired",
    });
  });

  it("drops an unsafe next and an unknown reason", () => {
    expect(validateAuthSearch({ next: "//evil", reason: "hacked" })).toEqual({
      next: undefined,
      reason: undefined,
    });
  });

  it("builds hrefs with only the given values", () => {
    expect(authHref("/sign-in")).toBe("/sign-in");
    expect(authHref("/sign-in", { reason: "signed-out" })).toBe("/sign-in?reason=signed-out");
    expect(authHref("/sign-in", { next: "/items/1?t=2", reason: "expired" })).toBe(
      "/sign-in?next=%2Fitems%2F1%3Ft%3D2&reason=expired",
    );
  });
});

function snapshot<Data>(state: Partial<QuerySnapshot<Data>>): QuerySnapshot<Data> {
  return { data: undefined, error: null, isPending: false, ...state };
}

function apiError(status: number, code: string): ApiError {
  return new ApiError({ status, title: code, code });
}

const pending = snapshot<never>({ isPending: true });
const setupDone = snapshot({ data: { required: false } });

describe("deriveSession", () => {
  it.each<{
    name: string;
    setup: QuerySnapshot<{ required: boolean }>;
    me: QuerySnapshot<Me>;
    expected: Partial<SessionState>;
  }>([
    { name: "loading setup", setup: pending, me: pending, expected: { status: "loading" } },
    { name: "loading me", setup: setupDone, me: pending, expected: { status: "loading" } },
    {
      name: "setup state before me answered",
      setup: snapshot({ data: { required: true } }),
      me: pending,
      expected: { status: "setupRequired" },
    },
    {
      name: "me says setup_required",
      setup: setupDone,
      me: snapshot({ error: apiError(401, "setup_required") }),
      expected: { status: "setupRequired" },
    },
    {
      name: "no session",
      setup: setupDone,
      me: snapshot({ error: apiError(401, "unauthenticated") }),
      expected: { status: "signedOut", authAvailable: true },
    },
    {
      name: "no profile",
      setup: setupDone,
      me: snapshot({ data: { ...fakeMe, currentProfile: null } }),
      expected: { status: "needsProfile" },
    },
    {
      name: "signed in with a profile",
      setup: setupDone,
      me: snapshot({ data: fakeMe }),
      expected: { status: "ready", me: fakeMe, authAvailable: true },
    },
    {
      name: "a server without accounts (501)",
      setup: snapshot({ error: apiError(501, "not_implemented") }),
      me: snapshot({ error: apiError(501, "not_implemented") }),
      expected: { status: "ready", authAvailable: false },
    },
    {
      name: "a failing me",
      setup: setupDone,
      me: snapshot({ error: apiError(500, "internal_error") }),
      expected: { status: "ready", authAvailable: false },
    },
    {
      name: "a malformed me",
      setup: setupDone,
      me: snapshot({ data: { productName: "x" } as unknown as Me }),
      expected: { status: "ready", authAvailable: false },
    },
  ])("derives $expected.status for $name", ({ setup, me, expected }) => {
    expect(deriveSession(setup, me)).toMatchObject(expected);
  });
});

function at(href: string): GuardLocation {
  const url = new URL(href, "http://app.test");
  return { pathname: url.pathname, href, next: url.searchParams.get("next") ?? undefined };
}

const states = {
  loading: { status: "loading", me: undefined, authAvailable: true },
  setupRequired: { status: "setupRequired", me: undefined, authAvailable: true },
  signedOut: { status: "signedOut", me: undefined, authAvailable: true },
  needsProfile: { status: "needsProfile", me: fakeMe, authAvailable: true },
  ready: { status: "ready", me: fakeMe, authAvailable: true },
} as const satisfies Record<string, SessionState>;

describe("guardRedirect", () => {
  it.each([
    { state: states.loading, href: "/items/1", expected: undefined },
    { state: states.setupRequired, href: "/items/1", expected: "/setup" },
    { state: states.setupRequired, href: "/sign-in", expected: "/setup" },
    { state: states.setupRequired, href: "/setup", expected: undefined },
    { state: states.setupRequired, href: "/setup/remote", expected: undefined },
    {
      state: states.signedOut,
      href: "/items/1?t=3",
      expected: "/sign-in?next=%2Fitems%2F1%3Ft%3D3",
    },
    { state: states.signedOut, href: "/", expected: "/sign-in" },
    { state: states.signedOut, href: "/setup", expected: "/sign-in" },
    {
      state: states.signedOut,
      href: "/profiles?next=%2Fitems%2F1",
      expected: "/sign-in?next=%2Fitems%2F1",
    },
    { state: states.signedOut, href: "/sign-in?next=%2Fitems%2F1", expected: undefined },
    { state: states.needsProfile, href: "/items/1", expected: "/profiles?next=%2Fitems%2F1" },
    {
      state: states.needsProfile,
      href: "/sign-in?next=%2Fitems%2F1",
      expected: "/profiles?next=%2Fitems%2F1",
    },
    { state: states.needsProfile, href: "/profiles", expected: undefined },
    { state: states.ready, href: "/items/1", expected: undefined },
    { state: states.ready, href: "/sign-in?next=%2Fitems%2F1", expected: "/items/1" },
    { state: states.ready, href: "/sign-in?next=%2F%2Fevil.example", expected: "/" },
    { state: states.ready, href: "/setup", expected: "/" },
    { state: states.ready, href: "/profiles", expected: "/" },
  ])("sends $state.status at $href to $expected", ({ state, href, expected }) => {
    expect(guardRedirect(state, at(href))).toBe(expected);
  });
});

describe("unauthorizedAction", () => {
  it.each([
    {
      method: "GET",
      schemaPath: "/api/v1/libraries",
      status: 401,
      code: "unauthenticated",
      expected: "signIn",
    },
    {
      method: "PUT",
      schemaPath: "/api/v1/me/password",
      status: 401,
      code: "unauthenticated",
      expected: "signIn",
    },
    {
      method: "GET",
      schemaPath: "/api/v1/home/rows",
      status: 401,
      code: "setup_required",
      expected: "setup",
    },
    {
      method: "GET",
      schemaPath: "/api/v1/home/rows",
      status: 403,
      code: "profile_required",
      expected: "profiles",
    },
    // A wrong current password is the dialog's business and never signs out.
    {
      method: "PUT",
      schemaPath: "/api/v1/me/password",
      status: 403,
      code: "invalid_credentials",
      expected: undefined,
    },
    {
      method: "POST",
      schemaPath: "/api/v1/libraries",
      status: 403,
      code: "forbidden",
      expected: undefined,
    },
    {
      method: "POST",
      schemaPath: "/api/v1/auth/session",
      status: 401,
      code: "invalid_credentials",
      expected: undefined,
    },
    {
      method: "DELETE",
      schemaPath: "/api/v1/auth/session",
      status: 401,
      code: "unauthenticated",
      expected: undefined,
    },
    {
      method: "GET",
      schemaPath: "/api/v1/me",
      status: 401,
      code: "unauthenticated",
      expected: undefined,
    },
    {
      method: "GET",
      schemaPath: "/api/v1/setup",
      status: 401,
      code: undefined,
      expected: undefined,
    },
    {
      method: "GET",
      schemaPath: "/api/v1/libraries",
      status: 501,
      code: "not_implemented",
      expected: undefined,
    },
  ])("maps $status $code of $method $schemaPath to $expected", ({ expected, ...facts }) => {
    expect(unauthorizedAction(facts)).toBe(expected);
  });

  it("reads bodies only of 401 and 403", () => {
    expect([401, 403, 200, 404, 501].map(mayAffectSession)).toEqual([
      true,
      true,
      false,
      false,
      false,
    ]);
  });
});

describe("credentials", () => {
  it.each([
    ["", "empty"],
    ["   ", "empty"],
    ["Anna", undefined],
    ["a".repeat(40), undefined],
    ["a".repeat(41), "tooLong"],
    // 40 emoji are 40 code points, as the server counts them.
    ["😀".repeat(40), undefined],
  ])("checks the name %j", (name, expected) => {
    expect(checkName(name)).toBe(expected);
  });

  it.each([
    ["1234567", "tooShort"],
    ["12345678", undefined],
    ["a".repeat(128), undefined],
    ["a".repeat(129), "tooLong"],
    // Four bytes each in UTF-8: two make the minimum, 33 exceed the maximum.
    ["😀😀", undefined],
    ["😀".repeat(33), "tooLong"],
  ])("checks the password %j", (password, expected) => {
    expect(checkNewPassword(password)).toBe(expected);
  });

  it("checks the repetition only for a valid password", () => {
    expect(checkRepeat("short", "other")).toBeUndefined();
    expect(checkRepeat("long enough", "different")).toBe("mismatch");
    expect(checkRepeat("long enough", "long enough")).toBeUndefined();
  });
});

describe("countdown and errors", () => {
  it.each([
    [0, "0:00"],
    [5, "0:05"],
    [60, "1:00"],
    [899.2, "15:00"],
    [-3, "0:00"],
  ])("formats %d seconds as %s", (seconds, expected) => {
    expect(formatCountdown(seconds)).toBe(expected);
  });

  it("rounds the remaining time up and never below zero", () => {
    expect(secondsUntil(10_500, 10_000)).toBe(1);
    expect(secondsUntil(10_000, 12_000)).toBe(0);
  });

  it("falls back to one minute when a 429 names no wait", () => {
    expect(
      throttleSeconds(new ApiError({ status: 429, title: "x", code: "too_many_attempts" })),
    ).toBe(60);
    expect(
      throttleSeconds(
        new ApiError({ status: 429, title: "x", code: "too_many_attempts", retryAfterSeconds: 7 }),
      ),
    ).toBe(7);
  });

  it("tells a lost connection from other failures", () => {
    expect(generalFailure(new ApiError({ status: 0, title: "x", code: "network_error" }))).toBe(
      "network",
    );
    expect(generalFailure(new Error("boom"))).toBe("generic");
  });

  it("builds the recovery commands from the product id", () => {
    expect(recoveryCommand("Anna")).toMatch(/^\S+ admin reset-password --name Anna$/u);
    expect(dockerRecoveryCommand()).toMatch(
      /^docker exec -it \S+ \S+ admin reset-password --name …$/u,
    );
  });
});
