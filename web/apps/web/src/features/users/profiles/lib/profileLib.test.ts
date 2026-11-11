import { ApiError, clientErrorCodes, type Profile } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { annaPrimary, max, movies, shows } from "../../test/fakeUsersServer";
import { addProfileHref, validateAccountSearch } from "./accountPaths";
import { initialPin, isValidPin, pinChange, pinDigits, pinProblem, type PinDraft } from "./pin";
import { isChosen, libraryChoices, toProfileAccess } from "./profileAccess";
import {
  newProfileDraft,
  profileDraft,
  profileDraftProblems,
  toProfileCreateBody,
  toProfileUpdateBody,
} from "./profileDraft";
import { profileProblem } from "./profileProblems";

function apiError(status: number, code: string, field?: string): ApiError {
  return new ApiError({ status, code, title: code, field });
}

const moviesOnly = libraryChoices([movies, shows], { all: false, libraryIds: [movies.id] });
const everything = libraryChoices([movies, shows], { all: true });

describe("PIN", () => {
  it("keeps only ASCII digits, at most four", () => {
    expect(pinDigits("12a3")).toBe("123");
    expect(pinDigits("123456")).toBe("1234");
    expect(pinDigits("١٢٣٤")).toBe("");
  });

  it.each([
    { value: "1234", valid: true },
    { value: "0000", valid: true },
    { value: "123", valid: false },
    { value: "12345", valid: false },
    { value: "12 4", valid: false },
    { value: "", valid: false },
  ])("accepts exactly four digits: '$value' is $valid", ({ value, valid }) => {
    expect(isValidPin(value)).toBe(valid);
  });

  it.each<{ pin: PinDraft; hasPin: boolean; problem: string | undefined }>([
    { pin: { action: "set", value: "" }, hasPin: false, problem: undefined },
    { pin: { action: "set", value: "12" }, hasPin: false, problem: "invalid" },
    { pin: { action: "set", value: "1234" }, hasPin: false, problem: undefined },
    { pin: { action: "set", value: "" }, hasPin: true, problem: "required" },
    { pin: { action: "set", value: "123" }, hasPin: true, problem: "invalid" },
    { pin: { action: "keep" }, hasPin: true, problem: undefined },
    { pin: { action: "remove" }, hasPin: true, problem: undefined },
  ])("checks $pin on a profile with PIN $hasPin", ({ pin, hasPin, problem }) => {
    expect(pinProblem(pin, hasPin)).toBe(problem);
  });

  it("starts untouched with a stored PIN and with an empty field without", () => {
    expect(initialPin(true)).toEqual({ action: "keep" });
    expect(initialPin(false)).toEqual({ action: "set", value: "" });
  });

  it("sends the new PIN, null to remove it and nothing to keep it", () => {
    expect(pinChange({ action: "set", value: "4321" })).toBe("4321");
    expect(pinChange({ action: "set", value: "" })).toBeUndefined();
    expect(pinChange({ action: "remove" })).toBeNull();
    expect(pinChange({ action: "keep" })).toBeUndefined();
  });
});

describe("library subset", () => {
  it("allows only the libraries of the account", () => {
    expect(moviesOnly.map((choice) => choice.allowed)).toEqual([true, false]);
    expect(everything.every((choice) => choice.allowed)).toBe(true);
    expect(libraryChoices([movies], { all: false }).map((choice) => choice.allowed)).toEqual([
      false,
    ]);
  });

  it("never ticks or sends a library outside the account's set", () => {
    const draft = { all: false, libraryIds: [movies.id, shows.id, "gone"] };
    const [moviesChoice, showsChoice] = moviesOnly;
    if (moviesChoice === undefined || showsChoice === undefined) {
      throw new Error("two choices expected");
    }
    expect(isChosen(draft, moviesChoice)).toBe(true);
    expect(isChosen(draft, showsChoice)).toBe(false);
    expect(toProfileAccess(draft, moviesOnly)).toEqual({ all: false, libraryIds: [movies.id] });
    expect(toProfileAccess(draft, everything)).toEqual({
      all: false,
      libraryIds: [movies.id, shows.id],
    });
  });

  it("sends 'all' as the account's set without a list", () => {
    expect(toProfileAccess({ all: true, libraryIds: [shows.id] }, moviesOnly)).toEqual({
      all: true,
    });
  });
});

describe("profile draft", () => {
  it("creates with the contract's defaults and leaves out an empty PIN", () => {
    const body = toProfileCreateBody({ ...newProfileDraft, name: "  Mia " }, everything);
    expect(body).toEqual({
      name: "Mia",
      avatarColor: "coral",
      contentLimit: { preset: "adults", maxAge: null, hideUnrated: false },
      libraryAccess: { all: true },
    });
  });

  it("creates with PIN, colour and a subset", () => {
    const body = toProfileCreateBody(
      {
        ...newProfileDraft,
        name: "Mia",
        avatarColor: "teal",
        pin: { action: "set", value: "2468" },
        access: { all: false, libraryIds: [shows.id, movies.id] },
      },
      moviesOnly,
    );
    expect(body.pin).toBe("2468");
    expect(body.avatarColor).toBe("teal");
    expect(body.libraryAccess).toEqual({ all: false, libraryIds: [movies.id] });
  });

  it("flags a missing name, a long name and a short PIN", () => {
    expect(profileDraftProblems(newProfileDraft, undefined)).toEqual({ name: "required" });
    expect(
      profileDraftProblems(
        { ...newProfileDraft, name: "x".repeat(41), pin: { action: "set", value: "1" } },
        undefined,
      ),
    ).toEqual({ name: "tooLong", pin: "invalid" });
    expect(
      profileDraftProblems({ ...profileDraft(max), pin: { action: "set", value: "" } }, max),
    ).toEqual({ pin: "required" });
  });

  it("updates only what changed", () => {
    const draft = profileDraft(max);
    expect(toProfileUpdateBody(draft, max, everything)).toEqual({});
    expect(
      toProfileUpdateBody(
        { ...draft, name: "Maxi", avatarColor: "pink", pin: { action: "remove" } },
        max,
        everything,
      ),
    ).toEqual({ name: "Maxi", avatarColor: "pink", pin: null });
    expect(
      toProfileUpdateBody(
        {
          ...draft,
          access: { all: false, libraryIds: [movies.id] },
          limit: { ...draft.limit, preset: "teens" },
        },
        max,
        everything,
      ),
    ).toEqual({
      libraryAccess: { all: false, libraryIds: [movies.id] },
      contentLimit: { preset: "teens", maxAge: 12, hideUnrated: true },
    });
  });

  it("sends only colour and PIN for a primary profile", () => {
    const primary: Profile = annaPrimary;
    const draft = {
      ...profileDraft(primary),
      name: "Someone else",
      avatarColor: "violet" as const,
      pin: { action: "set", value: "1357" } as const,
      access: { all: false, libraryIds: [] },
    };
    expect(profileDraftProblems({ ...draft, name: "" }, primary)).toEqual({});
    expect(toProfileUpdateBody(draft, primary, everything)).toEqual({
      avatarColor: "violet",
      pin: "1357",
    });
  });
});

describe("profileProblem", () => {
  it.each([
    { error: apiError(409, "name_taken"), place: "name", message: "nameTaken" },
    { error: apiError(409, "profile_limit"), place: "form", message: "profileLimit" },
    { error: apiError(409, "conflict"), place: "form", message: "primaryProfile" },
    { error: apiError(404, "not_found"), place: "form", message: "notFound" },
    { error: apiError(403, "forbidden"), place: "form", message: "forbidden" },
    { error: apiError(501, "not_implemented"), place: "form", message: "notImplemented" },
    { error: apiError(400, "invalid", "pin"), place: "pin", message: "pinInvalid" },
    { error: apiError(400, "invalid", "name"), place: "name", message: "nameInvalid" },
    {
      error: apiError(400, "invalid", "libraryAccess"),
      place: "libraryAccess",
      message: "accessInvalid",
    },
    {
      error: apiError(400, "invalid", "contentLimit"),
      place: "contentLimit",
      message: "limitInvalid",
    },
    { error: apiError(400, "invalid", "toString"), place: "form", message: "failed" },
    { error: apiError(0, clientErrorCodes.networkError), place: "form", message: "failed" },
    { error: new Error("boom"), place: "form", message: "failed" },
  ])("maps $error.code to $message", ({ error, place, message }) => {
    expect(profileProblem(error)).toEqual({ place, message });
  });
});

describe("account search", () => {
  it("keeps a profile target and drops anything else", () => {
    expect(validateAccountSearch({ profile: "new" })).toEqual({ profile: "new" });
    expect(validateAccountSearch({ profile: max.id })).toEqual({ profile: max.id });
    expect(validateAccountSearch({ profile: "" })).toEqual({ profile: undefined });
    expect(validateAccountSearch({ profile: 7 })).toEqual({ profile: undefined });
    expect(validateAccountSearch({ profile: "x".repeat(65) })).toEqual({ profile: undefined });
    expect(addProfileHref).toBe("/settings/account?profile=new");
  });
});
