import { describe, expect, it } from "vitest";

import {
  apiSessionFitsCore,
  clientProfileFitsApi,
  clientProfileKeysMatch,
  coreSessionFitsApi,
  hlsErrorFitsCore,
  sessionKeysMatch,
  streamInfoColorIsToken,
} from "./apiTypeChecks";

describe("apiTypeChecks", () => {
  it("proves the mirrored types compatible at compile time", () => {
    // The values only exist so that `tsc` checks their types; see apiTypeChecks.ts.
    expect([
      clientProfileFitsApi,
      ...clientProfileKeysMatch,
      apiSessionFitsCore,
      coreSessionFitsApi,
      ...sessionKeysMatch,
      hlsErrorFitsCore,
      streamInfoColorIsToken,
    ]).toEqual(Array.from({ length: 12 }, () => true));
  });
});
