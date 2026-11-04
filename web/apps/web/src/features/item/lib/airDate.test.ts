import { describe, expect, it } from "vitest";

import { formatAirDate } from "./airDate";

describe("formatAirDate", () => {
  it("formats the calendar day in the UI language", () => {
    expect(formatAirDate("2008-05-20", "en")).toBe("May 20, 2008");
    expect(formatAirDate("2008-05-20", "de")).toBe("20. Mai 2008");
  });

  it.each([undefined, "", "2008", "20.05.2008", "2008-05-20T10:00:00Z"])(
    "gives nothing for %s",
    (value) => {
      expect(formatAirDate(value, "en")).toBeUndefined();
    },
  );
});
