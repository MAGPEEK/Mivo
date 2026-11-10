import { describe, expect, it } from "vitest";

import { reportsProgress } from "./progressKinds";

describe("reportsProgress", () => {
  it.each(["movie", "episode", "audiobook"])("reports %s", (kind) => {
    expect(reportsProgress(kind)).toBe(true);
  });

  it.each(["track", "album", "artist", "photo", "show", "season", "extra", "unknown"])(
    "does not report %s",
    (kind) => {
      expect(reportsProgress(kind)).toBe(false);
    },
  );
});
