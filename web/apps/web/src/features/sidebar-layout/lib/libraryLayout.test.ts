import { describe, expect, it } from "vitest";

import {
  arrangeLibraries,
  moveLibraryBy,
  moveLibraryTo,
  parseStoredLayout,
  serializeLayout,
  toggleLibraryPinned,
  toLayout,
  type ArrangedLibrary,
  type LibraryLayout,
} from "./libraryLayout";

function arranged(spec: string): ArrangedLibrary[] {
  // "a b- c" = a pinned, b unpinned, c pinned.
  return spec
    .split(" ")
    .filter((token) => token !== "")
    .map((token) =>
      token.endsWith("-") ? { id: token.slice(0, -1), pinned: false } : { id: token, pinned: true },
    );
}

describe("arrangeLibraries", () => {
  it.each<{
    name: string;
    serverIds: string[];
    layout: LibraryLayout | undefined;
    expected: string;
  }>([
    {
      name: "no layout keeps server order, all pinned",
      serverIds: ["a", "b", "c"],
      layout: undefined,
      expected: "a b c",
    },
    {
      name: "saved order and pins apply",
      serverIds: ["a", "b", "c"],
      layout: { order: ["c", "a", "b"], unpinned: ["a"] },
      expected: "c a- b",
    },
    {
      name: "new libraries appear pinned at the end in server order",
      serverIds: ["a", "new1", "b", "new2"],
      layout: { order: ["b", "a"], unpinned: [] },
      expected: "b a new1 new2",
    },
    {
      name: "removed libraries drop out silently",
      serverIds: ["b"],
      layout: { order: ["a", "b", "c"], unpinned: ["c"] },
      expected: "b",
    },
    {
      name: "duplicate ids in the saved order count once",
      serverIds: ["a", "b"],
      layout: { order: ["b", "b", "a"], unpinned: [] },
      expected: "b a",
    },
    { name: "no libraries", serverIds: [], layout: { order: ["a"], unpinned: [] }, expected: "" },
  ])("$name", ({ serverIds, layout, expected }) => {
    expect(arrangeLibraries(serverIds, layout)).toEqual(arranged(expected));
  });
});

describe("moveLibraryBy", () => {
  it.each([
    { name: "up", start: "a b c", id: "b", offset: -1, expected: "b a c" },
    { name: "down", start: "a b c", id: "b", offset: 1, expected: "a c b" },
    {
      name: "up past an unpinned library",
      start: "a- b c",
      id: "b",
      offset: -1,
      expected: "b a- c",
    },
    { name: "first cannot move up", start: "a b", id: "a", offset: -1, expected: "a b" },
    { name: "last cannot move down", start: "a b", id: "b", offset: 1, expected: "a b" },
    { name: "unknown id changes nothing", start: "a b", id: "x", offset: 1, expected: "a b" },
  ])("$name", ({ start, id, offset, expected }) => {
    expect(moveLibraryBy(arranged(start), id, offset)).toEqual(arranged(expected));
  });
});

describe("moveLibraryTo", () => {
  it.each([
    {
      name: "drag down takes the target's place",
      start: "a b c",
      id: "a",
      target: "c",
      expected: "b c a",
    },
    {
      name: "drag up takes the target's place",
      start: "a b c",
      id: "c",
      target: "a",
      expected: "c a b",
    },
    {
      name: "skips unpinned libraries in between",
      start: "a x- b",
      id: "a",
      target: "b",
      expected: "x- b a",
    },
    { name: "drop on itself changes nothing", start: "a b", id: "a", target: "a", expected: "a b" },
    { name: "unknown target changes nothing", start: "a b", id: "a", target: "x", expected: "a b" },
    { name: "unknown source changes nothing", start: "a b", id: "x", target: "a", expected: "a b" },
  ])("$name", ({ start, id, target, expected }) => {
    expect(moveLibraryTo(arranged(start), id, target)).toEqual(arranged(expected));
  });
});

describe("toggleLibraryPinned", () => {
  it.each([
    { name: "unpins", start: "a b", id: "a", expected: "a- b" },
    { name: "pins", start: "a- b", id: "a", expected: "a b" },
    { name: "unknown id changes nothing", start: "a b", id: "x", expected: "a b" },
  ])("$name", ({ start, id, expected }) => {
    expect(toggleLibraryPinned(arranged(start), id)).toEqual(arranged(expected));
  });
});

describe("toLayout", () => {
  it("keeps the order and lists the unpinned libraries", () => {
    expect(toLayout(arranged("c a- b"))).toEqual({ order: ["c", "a", "b"], unpinned: ["a"] });
  });
});

describe("parseStoredLayout", () => {
  it("round-trips a serialized layout", () => {
    const layout: LibraryLayout = { order: ["b", "a"], unpinned: ["a"] };

    expect(parseStoredLayout(serializeLayout(layout))).toEqual(layout);
  });

  it.each([
    { name: "missing value", raw: null },
    { name: "empty string", raw: "" },
    { name: "invalid JSON", raw: "{order:" },
    { name: "JSON null", raw: "null" },
    { name: "an array", raw: "[]" },
    { name: "legacy layout without version", raw: '{"order":["a"],"pinned":{"a":true}}' },
    { name: "unknown future version", raw: '{"version":2,"order":["a"],"unpinned":[]}' },
    { name: "order not an array", raw: '{"version":1,"order":"a","unpinned":[]}' },
    { name: "non-string id", raw: '{"version":1,"order":[1],"unpinned":[]}' },
    { name: "unpinned missing", raw: '{"version":1,"order":["a"]}' },
  ])("falls back to defaults for $name", ({ raw }) => {
    expect(parseStoredLayout(raw)).toBeUndefined();
  });
});
