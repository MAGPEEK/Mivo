import { describe, expect, it } from "vitest";

import { childOrder, oppositeOrder, searchForOrder, validateItemSearch } from "./itemSearch";

describe("validateItemSearch", () => {
  it.each([
    { raw: { order: "desc" }, order: "desc" },
    { raw: { order: "asc" }, order: undefined },
    { raw: { order: "sideways" }, order: undefined },
    { raw: { order: 3 }, order: undefined },
    { raw: {}, order: undefined },
  ])("reads $raw as $order", ({ raw, order }) => {
    expect(validateItemSearch(raw)).toEqual({ order });
  });
});

describe("order helpers", () => {
  it("maps between search and order", () => {
    expect(childOrder({ order: "desc" })).toBe("desc");
    expect(childOrder({})).toBe("asc");
    expect(searchForOrder("desc")).toEqual({ order: "desc" });
    expect(searchForOrder("asc")).toEqual({ order: undefined });
    expect(oppositeOrder("asc")).toBe("desc");
    expect(oppositeOrder("desc")).toBe("asc");
  });
});
