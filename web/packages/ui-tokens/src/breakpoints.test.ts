import { describe, expect, it } from "vitest";

import { belowBreakpointQuery, breakpoints } from "./index";

describe("breakpoints", () => {
  it("grow from sm to md", () => {
    expect(breakpoints.sm).toBeLessThan(breakpoints.md);
  });

  it.each([
    { breakpoint: "sm", query: "(width < 40rem)" },
    { breakpoint: "md", query: "(width < 48rem)" },
  ] as const)("writes the $breakpoint query in rem like Tailwind", ({ breakpoint, query }) => {
    expect(belowBreakpointQuery(breakpoint)).toBe(query);
  });
});
