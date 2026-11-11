import { ApiError, clientErrorCodes, type Me } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeMe } from "../../../test/fakeApi";
import { annaProfile, lenaProfile, maxProfile } from "../test/fakeProfilesServer";
import { avatarBackgroundClass, avatarInitial } from "../../../shared/ui";
import { maxManagedProfiles } from "../../users";
import { applyPadKey, isCompletePin, movePadFocus, padKeyForKeyboard, padKeys } from "./pinPad";
import {
  canAddProfile,
  canChangePassword,
  needsPin,
  primaryProfile,
  rememberByDefault,
  suggestsOwnPin,
} from "./profileModel";
import { defaultLockoutSeconds, selectionFailure } from "./selectionFailure";

function apiError(status: number, code: string, retryAfterSeconds?: number): ApiError {
  return new ApiError({ status, code, title: code, retryAfterSeconds });
}

function me(overrides: Partial<Me>): Me {
  return { ...fakeMe, ...overrides };
}

describe("applyPadKey", () => {
  it.each([
    { buffer: "", key: 1, expected: "1" },
    { buffer: "12", key: 0, expected: "120" },
    { buffer: "1234", key: 5, expected: "1234" },
    { buffer: "123", key: "delete", expected: "12" },
    { buffer: "", key: "delete", expected: "" },
    { buffer: "12", key: "submit", expected: "12" },
  ] as const)("$buffer + $key = $expected", ({ buffer, key, expected }) => {
    expect(applyPadKey(buffer, key)).toBe(expected);
  });

  it("knows a whole PIN", () => {
    expect(isCompletePin("1234")).toBe(true);
    expect(isCompletePin("123")).toBe(false);
  });
});

describe("padKeyForKeyboard", () => {
  it.each([
    { key: "7", expected: 7 },
    { key: "0", expected: 0 },
    { key: "Backspace", expected: "delete" },
    { key: "Delete", expected: "delete" },
    { key: "Enter", expected: undefined },
    { key: "a", expected: undefined },
    { key: "ArrowLeft", expected: undefined },
  ])("$key -> $expected", ({ key, expected }) => {
    expect(padKeyForKeyboard(key)).toBe(expected);
  });
});

describe("movePadFocus", () => {
  it("has the phone keypad layout", () => {
    expect(padKeys).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, "delete", 0, "submit"]);
  });

  it.each([
    { index: 0, key: "ArrowRight", expected: 1 },
    { index: 2, key: "ArrowRight", expected: 2 },
    { index: 1, key: "ArrowLeft", expected: 0 },
    { index: 3, key: "ArrowLeft", expected: 3 },
    { index: 1, key: "ArrowDown", expected: 4 },
    { index: 10, key: "ArrowDown", expected: 10 },
    { index: 4, key: "ArrowUp", expected: 1 },
    { index: 1, key: "ArrowUp", expected: 1 },
    { index: 4, key: "Enter", expected: undefined },
  ])("$key from $index -> $expected", ({ index, key, expected }) => {
    expect(movePadFocus(index, key)).toBe(expected);
  });
});

describe("avatar", () => {
  it("maps every token colour and shows unknown ones in slate", () => {
    expect(avatarBackgroundClass("teal")).toBe("bg-avatar-teal");
    expect(avatarBackgroundClass("chartreuse")).toBe("bg-avatar-slate");
    expect(avatarBackgroundClass("toString")).toBe("bg-avatar-slate");
  });

  it("shows the first letter in upper case", () => {
    expect(avatarInitial(" lena")).toBe("L");
    expect(avatarInitial("Ölaf")).toBe("Ö");
    expect(avatarInitial("")).toBe("?");
  });
});

describe("selectionFailure", () => {
  it.each([
    { error: apiError(403, "pin_invalid"), expected: { kind: "wrongPin" } },
    { error: apiError(429, "profile_locked", 120), expected: { kind: "locked", seconds: 120 } },
    {
      error: apiError(429, "profile_locked"),
      expected: { kind: "locked", seconds: defaultLockoutSeconds },
    },
    { error: apiError(0, clientErrorCodes.networkError), expected: { kind: "network" } },
    { error: apiError(403, "forbidden"), expected: { kind: "generic" } },
    { error: new Error("boom"), expected: { kind: "generic" } },
  ])("$error.message -> $expected.kind", ({ error, expected }) => {
    expect(selectionFailure(error)).toEqual(expected);
  });
});

describe("profileModel", () => {
  it("asks for a PIN and forgets the choice by default only for PIN profiles", () => {
    expect(needsPin(maxProfile)).toBe(true);
    expect(rememberByDefault(maxProfile)).toBe(false);
    expect(needsPin(lenaProfile)).toBe(false);
    expect(rememberByDefault(lenaProfile)).toBe(true);
  });

  it("finds the primary profile", () => {
    expect(primaryProfile(me({ profiles: [maxProfile, annaProfile] }))).toBe(annaProfile);
    expect(primaryProfile(me({ profiles: [maxProfile] }))).toBeUndefined();
  });

  it("offers Add profile below the limit only", () => {
    const managed = Array.from({ length: maxManagedProfiles }, (_, index) => ({
      ...lenaProfile,
      id: String(index),
    }));
    expect(canAddProfile(me({ profiles: [annaProfile, maxProfile] }))).toBe(true);
    expect(canAddProfile(me({ profiles: [annaProfile, ...managed] }))).toBe(false);
    expect(canAddProfile(me({ profiles: [maxProfile] }))).toBe(false);
  });

  it("suggests a PIN on the primary profile once managed profiles exist", () => {
    const profiles = [annaProfile, maxProfile];
    expect(suggestsOwnPin(me({ currentProfile: annaProfile, profiles }))).toBe(true);
    expect(suggestsOwnPin(me({ currentProfile: annaProfile, profiles: [annaProfile] }))).toBe(
      false,
    );
    expect(suggestsOwnPin(me({ currentProfile: { ...annaProfile, hasPin: true }, profiles }))).toBe(
      false,
    );
    expect(suggestsOwnPin(me({ currentProfile: maxProfile, profiles }))).toBe(false);
    expect(suggestsOwnPin(me({ currentProfile: null, profiles }))).toBe(false);
  });

  it("lets only the primary profile change the account password", () => {
    expect(canChangePassword(annaProfile)).toBe(true);
    expect(canChangePassword(maxProfile)).toBe(false);
  });
});
