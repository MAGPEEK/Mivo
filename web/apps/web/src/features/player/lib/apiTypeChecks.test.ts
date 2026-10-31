import { describe, expect, it } from "vitest";

import {
  apiSessionFitsCore,
  clientProfileFitsApi,
  clientProfileKeysMatch,
  coreSessionFitsApi,
  hlsErrorFitsCore,
  sessionKeysMatch,
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
    ]).toEqual(Array.from({ length: 11 }, () => true));
  });
});
