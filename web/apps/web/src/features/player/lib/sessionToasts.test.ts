import { describe, expect, it } from "vitest";

import { multiTrackSession } from "../test/playerFixtures";
import { sessionToasts } from "./sessionToasts";

describe("sessionToasts", () => {
  it("says nothing for a session without the two codes", () => {
    expect(sessionToasts(multiTrackSession)).toEqual([]);
  });

  it("reports a burn-in the server could not do", () => {
    const session = {
      ...multiTrackSession,
      warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" } as const],
    };

    expect(sessionToasts(session)).toEqual(["burnInUnavailable"]);
  });

  it("reports a quality cap, whether it sits in the reasons or the warnings", () => {
    const reason = { code: "bitrate_cap", scope: "video" } as const;

    expect(sessionToasts({ ...multiTrackSession, reasons: [reason] })).toEqual(["bitRateCap"]);
    expect(sessionToasts({ ...multiTrackSession, warnings: [reason] })).toEqual(["bitRateCap"]);
  });

  it("reports both together", () => {
    const session = {
      ...multiTrackSession,
      reasons: [{ code: "bitrate_cap", scope: "video" } as const],
      warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" } as const],
    };

    expect(sessionToasts(session)).toEqual(["burnInUnavailable", "bitRateCap"]);
  });
});
