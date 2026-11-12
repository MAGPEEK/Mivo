import type { ExtraType, ItemSummary } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { firstTrailer, groupExtras } from "./extras";

function extra(id: string, extraType?: ExtraType): ItemSummary {
  return {
    id,
    libraryId: "library-1",
    kind: "extra",
    title: id,
    ...(extraType === undefined ? {} : { extraType }),
  };
}

describe("groupExtras", () => {
  it("groups by kind in the fixed order and keeps the server order inside a group", () => {
    const groups = groupExtras([
      extra("deleted", "deleted_scene"),
      extra("making-of", "featurette"),
      extra("teaser", "trailer"),
      extra("bts", "behind_the_scenes"),
      extra("trailer-2", "trailer"),
    ]);

    expect(groups.map((group) => [group.type, group.extras.map((item) => item.id)])).toEqual([
      ["trailer", ["teaser", "trailer-2"]],
      ["featurette", ["making-of"]],
      ["behind_the_scenes", ["bts"]],
      ["deleted_scene", ["deleted"]],
    ]);
  });

  it("puts extras without a known kind into the last group", () => {
    const unknown = { ...extra("mystery"), extraType: "future_kind" } as unknown as ItemSummary;

    expect(groupExtras([unknown, extra("plain")]).map((group) => group.type)).toEqual(["other"]);
  });

  it("returns no group without extras", () => {
    expect(groupExtras([])).toEqual([]);
  });
});

describe("firstTrailer", () => {
  it("finds the first trailer", () => {
    expect(
      firstTrailer([extra("a", "featurette"), extra("b", "trailer"), extra("c", "trailer")]),
    ).toMatchObject({ id: "b" });
  });

  it("is undefined without a trailer", () => {
    expect(firstTrailer([extra("a", "featurette")])).toBeUndefined();
  });
});
