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
};

interface ViewCase {
  readonly name: string;
  readonly item: ItemDetail;
  readonly expected: Partial<ItemView>;
}

function item(fields: Partial<ItemDetail>): ItemDetail {
  return { id: "item-1", libraryId: "library-1", title: "Title", files: [], ...fields };
}

const viewCases: readonly ViewCase[] = [
  {
    name: "a playable HDR movie",
    item: hdrMovie,
    expected: {
      title: "Big Buck Bunny",
      initial: "B",
      facts: ['duration:{"unit":"hours","hours":1,"minutes":30}', "4K"],
      chips: ["MKV", "HEVC 10-bit HDR10", "AC-3 5.1", "SRT"],
      file: { name: "Big.Buck.Bunny.mkv", size: "276.1 MB" },
      play: { kind: "playable", fileId: "file-1" },
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
      initial: "É",
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
    expected: { initial: "🎬", facts: ["1080p"], chips: ["H.264"] },
  },
  {
    name: "video without a height and an empty title",
    item: item({
      title: "",
      files: [mediaFile({ durationSeconds: 42, streams: [stream({ kind: "video" })] })],
    }),
    expected: { initial: "", facts: ['duration:{"unit":"seconds","seconds":42}'] },
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
