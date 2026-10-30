import { describe, expect, it } from "vitest";

import { mediaFile, stream } from "../test/fakeItemServer";
import { audioTracks, subtitleTracks } from "./tracks";

const file = mediaFile({
  streams: [
    stream({ kind: "video", codec: "h264" }),
    stream({ kind: "audio", codec: "ac3", channels: 6, language: "ger", default: true }),
    stream({ kind: "audio", codec: "aac", title: "Commentary" }),
    stream({ kind: "subtitle", codec: "subrip", language: "eng", forced: true, title: "" }),
  ],
});

describe("track lists", () => {
  it("lists the audio tracks in container order with their labels and flags", () => {
    expect(audioTracks(file)).toEqual([
      {
        index: 1,
        language: "ger",
        title: undefined,
        format: "AC-3 5.1",
        isDefault: true,
        isForced: false,
      },
      {
        index: 2,
        language: undefined,
        title: "Commentary",
        format: "AAC",
        isDefault: false,
        isForced: false,
      },
    ]);
  });

  it("lists the subtitle tracks and treats an empty title as none", () => {
    expect(subtitleTracks(file)).toEqual([
      {
        index: 3,
        language: "eng",
        title: undefined,
        format: "SRT",
        isDefault: false,
        isForced: true,
      },
    ]);
  });

  it("returns no tracks for a file without streams", () => {
    expect(audioTracks(mediaFile())).toEqual([]);
    expect(subtitleTracks(mediaFile())).toEqual([]);
  });
});
