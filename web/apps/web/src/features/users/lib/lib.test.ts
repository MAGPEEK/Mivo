import { ApiError, type ContentLimit, type User } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  contentLimitDraft,
  noContentLimit,
  sameContentLimit,
  toContentLimit,
  withPreset,
  type ContentLimitDraft,
} from "./contentLimit";
import {
  libraryAccessDraft,
  sameLibraryAccess,
  toLibraryAccess,
  withLibrary,
} from "./libraryAccess";
import { formatRelativeTime } from "./relativeTime";
import { draftProblems, newUserDraft, toCreateBody, toUpdateBody, userDraft } from "./userDraft";
import { isNotImplemented, userProblem } from "./userProblems";

const movies = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c";
const shows = "9c8b7a6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d";

const ben: User = {
  id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",
  name: "Ben",
  role: "user",
  status: "active",
  createdAt: "2026-11-01T10:00:00Z",
  libraryAccess: { all: false, libraryIds: [movies] },
  contentLimit: { preset: "teens", maxAge: 12, hideUnrated: true },
  profileCount: 1,
};

describe("contentLimitDraft", () => {
  const cases: readonly (readonly [string, ContentLimit, ContentLimitDraft])[] = [
    [
      "no limit",
      { preset: "adults", maxAge: null, hideUnrated: false },
      { preset: "adults", customAge: 16, hideUnrated: false },
    ],
    [
      "young children",
      { preset: "youngChildren", maxAge: 0, hideUnrated: true },
      { preset: "youngChildren", customAge: 0, hideUnrated: true },
    ],
    [
      "custom 16",
      { preset: "custom", maxAge: 16, hideUnrated: false },
      { preset: "custom", customAge: 16, hideUnrated: false },
    ],
    [
      "custom 6 derives the children preset",
      { preset: "custom", maxAge: 6, hideUnrated: true },
      { preset: "children", customAge: 6, hideUnrated: true },
    ],
    [
      "an age this client does not know is custom",
      { preset: "custom", maxAge: 14 as ContentLimit["maxAge"], hideUnrated: true },
      { preset: "custom", customAge: 16, hideUnrated: true },
    ],
  ];
  it.each(cases)("%s", (_name, limit, expected) => {
    expect(contentLimitDraft(limit)).toEqual(expected);
  });
});

describe("content limit editing", () => {
  it("sends a preset that matches maxAge", () => {
    expect(toContentLimit({ preset: "custom", customAge: 18, hideUnrated: true })).toEqual({
      preset: "custom",
      maxAge: 18,
      hideUnrated: true,
    });
    expect(toContentLimit(noContentLimit)).toEqual({
      preset: "adults",
      maxAge: null,
      hideUnrated: false,
    });
  });

  it("turns hide unrated on for a limit and off for no limit", () => {
    const teens = withPreset(noContentLimit, "teens");
    expect(teens.hideUnrated).toBe(true);
    const children = withPreset({ ...teens, hideUnrated: false }, "children");
    expect(children.hideUnrated).toBe(false);
    expect(withPreset(teens, "adults").hideUnrated).toBe(false);
  });

  it("compares by the limit sent", () => {
    const custom12: ContentLimitDraft = { preset: "custom", customAge: 12, hideUnrated: true };
    const teens: ContentLimitDraft = { preset: "teens", customAge: 16, hideUnrated: true };
    expect(sameContentLimit(custom12, teens)).toBe(true);
    expect(sameContentLimit(teens, { ...teens, hideUnrated: false })).toBe(false);
  });
});

describe("library access", () => {
  it("adds and removes a library", () => {
    const draft = libraryAccessDraft({ all: false });
    const both = withLibrary(withLibrary(draft, movies, true), shows, true);
    expect(both.libraryIds).toEqual([movies, shows]);
    expect(withLibrary(both, movies, false).libraryIds).toEqual([shows]);
  });

  it("sends only libraries that still exist", () => {
    const draft = { all: false, libraryIds: [movies, "gone"] };
    expect(toLibraryAccess(draft, [movies, shows])).toEqual({ all: false, libraryIds: [movies] });
    expect(toLibraryAccess({ ...draft, all: true }, [movies])).toEqual({ all: true });
  });

  it("compares sets, not order", () => {
    const left = { all: false, libraryIds: [movies, shows] };
    expect(sameLibraryAccess(left, { all: false, libraryIds: [shows, movies] })).toBe(true);
    expect(sameLibraryAccess(left, { all: false, libraryIds: [movies] })).toBe(false);
    expect(sameLibraryAccess(left, { all: true, libraryIds: [movies, shows] })).toBe(false);
    expect(
      sameLibraryAccess({ all: true, libraryIds: [] }, { all: true, libraryIds: [movies] }),
    ).toBe(true);
  });
});

describe("draftProblems", () => {
  const cases = [
    ["empty name", { name: "  " }, true, { name: "required", password: "required" }],
    ["long name", { name: "x".repeat(41), password: "longenough" }, true, { name: "tooLong" }],
    ["short password", { name: "Ben", password: "short" }, true, { password: "tooShort" }],
    ["long password", { name: "Ben", password: "ü".repeat(65) }, true, { password: "tooLong" }],
    ["edit without new password", { name: "Ben" }, false, {}],
    ["valid", { name: "Ben", password: "longenough" }, true, {}],
  ] as const;
  it.each(cases)("%s", (_name, change, creating, expected) => {
    expect(draftProblems({ ...newUserDraft, ...change }, creating)).toEqual(expected);
  });
});

describe("request bodies", () => {
  it("creates a user with access and limit, an admin without a library list", () => {
    const draft = {
      ...newUserDraft,
      name: " Ben ",
      password: "longenough",
      access: { all: false, libraryIds: [movies] },
    };
    expect(toCreateBody(draft, [movies])).toEqual({
      name: "Ben",
      password: "longenough",
      role: "user",
      contentLimit: { preset: "adults", maxAge: null, hideUnrated: false },
      libraryAccess: { all: false, libraryIds: [movies] },
    });
    expect(toCreateBody({ ...draft, role: "admin" }, [movies])).not.toHaveProperty("libraryAccess");
  });

  it("updates only what changed", () => {
    const original = userDraft(ben);
    expect(toUpdateBody(original, original, [movies])).toEqual({});
    expect(
      toUpdateBody({ ...original, name: "Benno ", disabled: true }, original, [movies]),
    ).toEqual({
      name: "Benno",
      status: "disabled",
    });
    const limit = { ...original.limit, preset: "children" as const };
    expect(toUpdateBody({ ...original, limit }, original, [movies])).toEqual({
      contentLimit: { preset: "children", maxAge: 6, hideUnrated: true },
    });
  });

  it("sends access with a demotion and none with a promotion", () => {
    const admin = userDraft({ ...ben, role: "admin", libraryAccess: { all: true } });
    expect(toUpdateBody({ ...admin, role: "user" }, admin, [movies])).toEqual({
      role: "user",
      libraryAccess: { all: true },
    });
    const user = userDraft(ben);
    expect(toUpdateBody({ ...user, role: "admin" }, user, [movies])).toEqual({ role: "admin" });
    expect(
      toUpdateBody({ ...user, disabled: false, access: { all: true, libraryIds: [] } }, user, [
        movies,
      ]),
    ).toEqual({ libraryAccess: { all: true } });
    expect(toUpdateBody(userDraft({ ...ben, status: "disabled" }), user, [])).toEqual({
      status: "disabled",
    });
  });
});

function apiError(status: number, code: string, field?: string): ApiError {
  return new ApiError({ status, title: code, code, field });
}

describe("userProblem", () => {
  const cases = [
    ["name taken", apiError(409, "name_taken"), { place: "name", message: "nameTaken" }],
    ["last admin", apiError(409, "last_admin"), { place: "form", message: "lastAdmin" }],
    ["not found", apiError(404, "not_found"), { place: "form", message: "notFound" }],
    ["invalid name", apiError(400, "invalid", "name"), { place: "name", message: "nameInvalid" }],
    [
      "invalid password",
      apiError(400, "invalid", "password"),
      { place: "password", message: "passwordInvalid" },
    ],
    [
      "invalid access",
      apiError(400, "invalid", "libraryAccess"),
      { place: "libraryAccess", message: "accessInvalid" },
    ],
    [
      "invalid limit",
      apiError(400, "invalid", "contentLimit"),
      { place: "contentLimit", message: "limitInvalid" },
    ],
    ["invalid other field", apiError(400, "invalid", "x"), { place: "form", message: "failed" }],
    ["forbidden", apiError(403, "forbidden"), { place: "form", message: "forbidden" }],
    [
      "not implemented",
      apiError(501, "not_implemented"),
      { place: "form", message: "notImplemented" },
    ],
    ["unknown code", apiError(500, "internal_error"), { place: "form", message: "failed" }],
    ["not an API error", new Error("boom"), { place: "form", message: "failed" }],
  ] as const;
  it.each(cases)("%s", (_name, error, expected) => {
    expect(userProblem(error)).toEqual(expected);
  });

  it("recognises 501 by status or code", () => {
    expect(isNotImplemented(apiError(501, "x"))).toBe(true);
    expect(isNotImplemented(apiError(500, "not_implemented"))).toBe(true);
    expect(isNotImplemented(new Error("x"))).toBe(false);
  });
});

describe("formatRelativeTime", () => {
  const now = new Date("2026-11-06T12:00:00Z");
  const cases = [
    ["now", "2026-11-06T11:59:40Z", "en", "now"],
    ["minutes", "2026-11-06T11:55:00Z", "en", "5 minutes ago"],
    ["hours", "2026-11-06T09:00:00Z", "en", "3 hours ago"],
    ["yesterday", "2026-11-05T12:00:00Z", "en", "yesterday"],
    ["weeks", "2026-10-23T12:00:00Z", "en", "2 weeks ago"],
    ["months", "2026-08-06T12:00:00Z", "en", "3 months ago"],
    ["years", "2024-11-06T12:00:00Z", "en", "2 years ago"],
    ["German", "2026-11-05T12:00:00Z", "de", "gestern"],
  ] as const;
  it.each(cases)("%s", (_name, when, language, expected) => {
    expect(formatRelativeTime(new Date(when), now, language)).toBe(expected);
  });
});
