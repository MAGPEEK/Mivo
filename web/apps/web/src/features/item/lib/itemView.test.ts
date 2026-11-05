import type { ItemDetail } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { hdrMovie, mediaFile, stream, unreadableItem } from "../test/fakeItemServer";
import { toItemView, type ItemView, type ItemViewTexts } from "./itemView";

// Texts that show their inputs, so the table checks what the view asks to be translated.
const texts: ItemViewTexts = {
  language: "en",
  duration: (parts) => `duration:${JSON.stringify(parts)}`,
  unknownLanguage: "unknown",
  unavailable: (reason) => `unavailable:${reason}`,
  seasons: (count) => `${String(count)} seasons`,
  episodes: (count) => `${String(count)} episodes`,
  trackCount: (count) => `${String(count)} tracks`,
  authors: (names) => `By ${names}`,
  narrators: (names) => `Narrated by ${names}`,
  chapterCount: (count) => `${String(count)} chapters`,
};

interface ViewCase {
  readonly name: string;
  readonly item: ItemDetail;
  readonly expected: Partial<ItemView>;
}

function item(fields: Partial<ItemDetail>): ItemDetail {
  return {
    id: "item-1",
    libraryId: "library-1",
    kind: "movie",
    title: "Title",
    files: [],
    ...fields,
  };
}

const viewCases: readonly ViewCase[] = [
  {
    name: "a playable HDR movie",
    item: hdrMovie,
    expected: {
      title: "Big Buck Bunny",
      poster: undefined,
      facts: ['duration:{"unit":"hours","hours":1,"minutes":30}', "4K"],
      chips: ["MKV", "HEVC 10-bit HDR10", "AC-3 5.1", "SRT"],
      file: { name: "Big.Buck.Bunny.mkv", size: "276.1 MB" },
      play: { kind: "playable", itemId: "item-1", fileId: "file-1" },
    },
  },
  {
    name: "a file whose probing failed",
    item: unreadableItem,
    expected: {
      facts: [],
      chips: [],
      audioTracks: [],
      subtitleTracks: [],
      file: { name: "broken clip.avi", size: "512 byte" },
      play: { kind: "unavailable", explanation: "unavailable:probeFailed" },
    },
  },
  {
    name: "an item without files",
    item: item({ title: "  émile" }),
    expected: {
      facts: [],
      chips: [],
      file: undefined,
      audioTracks: [],
      subtitleTracks: [],
      play: { kind: "unavailable", explanation: "unavailable:noFile" },
    },
  },
  {
    name: "a title starting with an emoji and a letterboxed film without duration",
    item: item({
      title: "🎬 Clip",
      files: [
        mediaFile({
          streams: [stream({ kind: "video", codec: "h264", width: 1920, height: 800 })],
        }),
      ],
    }),
    expected: { facts: ["1080p"], chips: ["H.264"] },
  },
  {
    name: "video without a height and an empty title",
    item: item({
      title: "",
      files: [mediaFile({ durationSeconds: 42, streams: [stream({ kind: "video" })] })],
    }),
    expected: { facts: ['duration:{"unit":"seconds","seconds":42}'] },
  },
  {
    name: "a movie with a poster",
    item: item({ artwork: { poster: { id: "poster-1", tag: "tag-1" } } }),
    expected: { poster: { id: "poster-1", tag: "tag-1" } },
  },
  {
    name: "an episode with a still",
    item: item({ kind: "episode", artwork: { thumb: { id: "still-1", tag: "tag-1" } } }),
    expected: { still: { id: "still-1", tag: "tag-1" } },
  },
  {
    name: "an album with a square cover, year and tracks",
    item: item({
      kind: "album",
      year: 2019,
      childCount: 4,
      artwork: { square: { id: "cover-1", tag: "tag-1" } },
    }),
    expected: { square: { id: "cover-1", tag: "tag-1" }, facts: ["2019", "4 tracks"] },
  },
  {
    name: "an audiobook of two parts names its people, whole length and chapters",
    item: item({
      kind: "audiobook",
      year: 2021,
      people: [
        { name: "Ada Inkwell", role: "author" },
        { name: "Bram Quill", role: "author" },
        { name: "Nora Vale", role: "narrator" },
        { name: "Someone Else", role: "actor" },
      ],
      files: [
        mediaFile({ id: "part-1", durationSeconds: 1_800 }),
        mediaFile({ id: "part-2", durationSeconds: 1_900 }),
      ],
      chapters: [
        { title: "One", startMs: 0, endMs: 1_800_000, fileId: "part-1" },
        { title: "Two", startMs: 0, endMs: 1_900_000, fileId: "part-2" },
      ],
      artwork: { poster: { id: "cover-2", tag: "tag-2" } },
    }),
    expected: {
      poster: { id: "cover-2", tag: "tag-2" },
      facts: [
        "By Ada Inkwell and Bram Quill",
        "Narrated by Nora Vale",
        "2021",
        `duration:${JSON.stringify({ unit: "hours", hours: 1, minutes: 1 })}`,
        "2 chapters",
      ],
    },
  },
  {
    name: "an audiobook without people, length or chapters",
    item: item({ kind: "audiobook", files: [mediaFile()] }),
    expected: { facts: [] },
  },
  {
    name: "a movie with year, summary and genres",
    item: item({ year: 2008, summary: "A rabbit.", genres: ["Animation", "Comedy"] }),
    expected: {
      kind: "movie",
      facts: ["2008"],
      summary: "A rabbit.",
      genres: ["Animation", "Comedy"],
    },
  },
  {
    name: "a show counts seasons and episodes and plays its play item",
    item: item({
      kind: "show",
      year: 2011,
      childCount: 2,
      leafCount: 18,
      playItemId: "episode-1",
    }),
    expected: {
      facts: ["2011", "2 seasons", "18 episodes"],
      play: { kind: "playable", itemId: "episode-1" },
    },
  },
  {
    name: "a show without a play item",
    item: item({ kind: "show" }),
    expected: { play: { kind: "unavailable", explanation: "unavailable:noEpisodes" } },
  },
  {
    name: "a season counts episodes and links its siblings and ancestors",
    item: item({
      kind: "season",
      childCount: 9,
      previousId: "season-0",
      nextId: "season-2",
      playItemId: "episode-1",
      ancestors: [{ id: "show-1", kind: "show", title: "The Show" }],
    }),
    expected: {
      facts: ["9 episodes"],
      previousId: "season-0",
      nextId: "season-2",
      ancestors: [{ id: "show-1", kind: "show", title: "The Show" }],
    },
  },
  {
    name: "an episode with an air date",
    item: item({ kind: "episode", releaseDate: "2008-05-20" }),
    expected: { airDate: "May 20, 2008" },
  },
];

describe("toItemView", () => {
  it.each(viewCases)("maps $name", ({ item: source, expected }) => {
    expect(toItemView(source, texts)).toMatchObject(expected);
  });

  it("names track languages in the UI language and falls back to the unknown text", () => {
    const view = toItemView(hdrMovie, { ...texts, language: "de" });

    expect(view.audioTracks.map((track) => track.languageName)).toEqual(["Deutsch", "Englisch"]);
    expect(view.subtitleTracks.map((track) => track.languageName)).toEqual(["Englisch", "unknown"]);
    expect(view.file?.size).toBe("276,1 MB");
  });
});
