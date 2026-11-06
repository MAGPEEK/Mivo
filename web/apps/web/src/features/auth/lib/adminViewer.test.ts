import type { Me } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeMe } from "../../../test/fakeApi";
import { isAdminViewer } from "./adminViewer";
import type { SessionState } from "./sessionStatus";

const userMe: Me = {
  ...fakeMe,
  account: { ...fakeMe.account, role: "user" },
  currentProfile:
    fakeMe.currentProfile === null ? null : { ...fakeMe.currentProfile, isAdmin: false },
};

describe("isAdminViewer", () => {
  const cases: readonly (readonly [string, SessionState, boolean])[] = [
    ["admin profile", { status: "ready", me: fakeMe, authAvailable: true }, true],
    ["user profile", { status: "ready", me: userMe, authAvailable: true }, false],
    [
      "no profile yet",
      { status: "needsProfile", me: { ...fakeMe, currentProfile: null }, authAvailable: true },
      false,
    ],
    ["signed out", { status: "signedOut", me: undefined, authAvailable: true }, false],
    ["server without accounts", { status: "ready", me: undefined, authAvailable: false }, true],
    ["still loading", { status: "loading", me: undefined, authAvailable: false }, false],
  ];
  it.each(cases)("%s", (_name, session, expected) => {
    expect(isAdminViewer(session)).toBe(expected);
  });
});
