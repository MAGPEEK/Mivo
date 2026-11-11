import type { ItemSummary } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { toChildRow, type ChildRowTexts } from "./childRow";

const texts: ChildRowTexts = {
  seasonName: (number) => `Season ${String(number)}`,
  specials: "Specials",
  seasonCaption: (name, count) => `${name} · ${String(count)} episodes`,
  episodeNumber: ({ first, last }) =>
    first === last ? `Episode ${String(first)}` : `Episodes ${String(first)}–${String(last)}`,
  duration: (parts) => `duration:${parts.unit}`,
};

function summary(fields: Partial<ItemSummary>): ItemSummary {
  return { id: "child-1", libraryId: "library-1", kind: "episode", title: "Pilot", ...fields };
}

describe("toChildRow", () => {
  it.each([
    {
      name: "a numbered season",
      fields: { kind: "season", number: 1, childCount: 9 },
      title: "Season 1 · 9 episodes",
    },
    {
      name: "the specials",
      fields: { kind: "season", number: 0, childCount: 3 },
      title: "Specials · 3 episodes",
    },
    {
      name: "a season without a number",
      fields: { kind: "season", title: "Extras", childCount: 1 },
      title: "Extras · 1 episodes",
    },
  ] as const)("captions $name", ({ fields, title }) => {
    const row = toChildRow(summary(fields), texts);

    expect(row.title).toBe(title);
    expect(row.label).toBe("");
  });

  it("uses the season poster and the episode still", () => {
    const poster = { id: "p", tag: "t" };
    const thumb = { id: "s", tag: "t" };

    expect(toChildRow(summary({ kind: "season", poster, thumb }), texts).image).toEqual(poster);
    expect(toChildRow(summary({ poster, thumb }), texts).image).toEqual(thumb);
  });

  it.each([
    { fields: { number: 8 }, label: "Episode 8" },
    { fields: { number: 1, numberEnd: 2 }, label: "Episodes 1–2" },
    { fields: {}, label: "" },
  ])("labels an episode $fields", ({ fields, label }) => {
    expect(toChildRow(summary(fields), texts)).toMatchObject({ title: "Pilot", label });
  });

  it("shows the runtime of the primary file when known", () => {
    const media = { available: true, durationSeconds: 2700 };

    expect(toChildRow(summary({ media }), texts).runtime).toBe("duration:minutes");
    expect(toChildRow(summary({}), texts).runtime).toBe("");
  });

  it("carries the watch badges: the count of a season, the progress of an episode", () => {
    const media = { available: true, durationSeconds: 2700 };
    const season = summary({
      kind: "season",
      viewerState: { watched: false, positionSeconds: 0, playCount: 0, unwatchedCount: 6 },
    });
    const episode = summary({
      media,
      viewerState: { watched: false, positionSeconds: 1350, playCount: 0 },
    });

    expect(toChildRow(season, texts).watch).toMatchObject({ unwatchedCount: 6, watched: false });
    expect(toChildRow(episode, texts).watch).toMatchObject({ progressPercent: 50 });
    expect(toChildRow(summary({ media }), texts).watch).toEqual({
      watched: false,
      progressPercent: undefined,
      unwatchedCount: undefined,
    });
  });
});
