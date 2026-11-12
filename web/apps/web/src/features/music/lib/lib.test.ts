import type { ItemSummary } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { toAlbumCardModel } from "./albumCard";
import { sortAlbumsByYear } from "./artistAlbums";
import {
  firstPlayableTrackId,
  groupByDisc,
  hasSeveralDiscs,
  playQueueIds,
  shuffleStartId,
  toTrackRowModel,
} from "./albumTracks";

function track(fields: Partial<ItemSummary> & { readonly id: string }): ItemSummary {
  return {
    libraryId: "library-1",
    kind: "track",
    title: `Track ${fields.id}`,
    media: { available: true, durationSeconds: 192.5 },
    ...fields,
  };
}

// A track the server lists without a media summary.
function withoutMedia(id: string): ItemSummary {
  return { id, libraryId: "library-1", kind: "track", title: `Track ${id}` };
}

describe("toAlbumCardModel", () => {
  it.each([
    {
      name: "an album with cover, year and artist",
      album: {
        id: "album-1",
        libraryId: "library-1",
        kind: "album",
        title: "Square Roots",
        year: 2021,
        parentId: "artist-1",
        square: { id: "cover-1", tag: "t" },
        poster: { id: "poster-1", tag: "t" },
      },
      expected: {
        id: "album-1",
        title: "Square Roots",
        year: "2021",
        cover: { id: "cover-1", tag: "t" },
        artistId: "artist-1",
      },
    },
    {
      name: "an album without cover, year or artist",
      album: { id: "album-2", libraryId: "library-1", kind: "album", title: "Loose" },
      expected: { id: "album-2", title: "Loose", year: "", cover: undefined, artistId: undefined },
    },
  ] satisfies readonly { name: string; album: ItemSummary; expected: object }[])(
    "maps $name",
    ({ album, expected }) => {
      expect(toAlbumCardModel(album)).toEqual(expected);
    },
  );
});

describe("toTrackRowModel", () => {
  it.each([
    {
      name: "a numbered, playable track",
      source: track({ id: "1", number: 3, title: "Allegro" }),
      expected: { id: "1", title: "Allegro", number: "3", duration: "3:12", available: true },
    },
    {
      name: "a track without number and duration",
      source: track({ id: "2", title: "Hidden", media: { available: true } }),
      expected: { id: "2", title: "Hidden", number: "", duration: "", available: true },
    },
    {
      name: "a track whose file could not be probed",
      source: track({ id: "3", media: { available: false } }),
      expected: { available: false },
    },
    {
      name: "a track without media summary",
      source: withoutMedia("4"),
      expected: { available: false, duration: "" },
    },
  ])("maps $name", ({ source, expected }) => {
    expect(toTrackRowModel(source)).toMatchObject(expected);
  });
});

describe("sortAlbumsByYear", () => {
  function album(id: string, year?: number): ItemSummary {
    return {
      id,
      libraryId: "library-1",
      kind: "album",
      title: id,
      ...(year === undefined ? {} : { year }),
    };
  }

  it("lists the oldest first, undated albums last, equal years in server order", () => {
    const sorted = sortAlbumsByYear([
      album("undated"),
      album("new", 2021),
      album("b", 2005),
      album("a", 2005),
    ]);

    expect(sorted.map((item) => item.id)).toEqual(["b", "a", "new", "undated"]);
  });

  it("keeps only albums", () => {
    const other: ItemSummary = { id: "t", libraryId: "library-1", kind: "track", title: "t" };

    expect(sortAlbumsByYear([other, album("a", 2000)]).map((item) => item.id)).toEqual(["a"]);
  });
});

describe("playQueueFrom", () => {
  const tracks = [
    track({ id: "a", discNumber: 1, number: 1 }),
    track({ id: "gone", discNumber: 1, number: 2, media: { available: false } }),
    track({ id: "b", discNumber: 2, number: 1 }),
  ];

  it("lists the playable tracks in album order", () => {
    expect(playQueueIds(tracks)).toEqual(["a", "b"]);
  });

  it("starts the shuffle at a track the random fraction picks", () => {
    expect(shuffleStartId(["a", "b", "c"], 0)).toBe("a");
    expect(shuffleStartId(["a", "b", "c"], 0.99)).toBe("c");
    expect(shuffleStartId([], 0.5)).toBeUndefined();
  });
});

describe("groupByDisc", () => {
  it("keeps the server order and starts a group at every disc change", () => {
    const groups = groupByDisc([
      track({ id: "a", discNumber: 1 }),
      track({ id: "b", discNumber: 1 }),
      track({ id: "c", discNumber: 2 }),
    ]);

    expect(groups.map((group) => group.disc)).toEqual([1, 2]);
    expect(groups.map((group) => group.tracks.map((row) => row.id))).toEqual([["a", "b"], ["c"]]);
    expect(hasSeveralDiscs(groups)).toBe(true);
  });

  it("counts a track without a disc tag as disc 1, like the server's order", () => {
    const groups = groupByDisc([
      track({ id: "a", discNumber: 1, number: 1 }),
      track({ id: "b", number: 2 }),
      track({ id: "c", discNumber: 2, number: 1 }),
    ]);

    expect(groups.map((group) => group.tracks.map((row) => row.id))).toEqual([["a", "b"], ["c"]]);
  });

  it.each([
    { name: "no tracks", tracks: [] },
    {
      name: "one disc",
      tracks: [track({ id: "a", discNumber: 1 }), track({ id: "b", discNumber: 1 })],
    },
    { name: "untagged tracks", tracks: [track({ id: "a" }), track({ id: "b" })] },
  ])("names no disc for $name", ({ tracks }) => {
    expect(hasSeveralDiscs(groupByDisc(tracks))).toBe(false);
  });
});

describe("firstPlayableTrackId", () => {
  it.each([
    {
      name: "skips tracks that cannot be played",
      tracks: [track({ id: "a", media: { available: false } }), track({ id: "b" })],
      expected: "b",
    },
    { name: "finds none in an empty album", tracks: [], expected: undefined },
    {
      name: "finds none when no track can be played",
      tracks: [withoutMedia("a")],
      expected: undefined,
    },
  ])("$name", ({ tracks, expected }) => {
    expect(firstPlayableTrackId(tracks)).toBe(expected);
  });
});
