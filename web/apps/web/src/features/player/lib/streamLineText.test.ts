import type { StreamLine } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import { streamLineParts } from "./streamLineText";

const videoLine: StreamLine = {
  kind: "video",
  kindKey: "streamInfo.streamKind.video",
  sourceIndex: 0,
  action: "transcode",
  actionKey: "streamInfo.action.transcode",
  sourceCodec: "HEVC",
  targetCodec: "H.264",
  targetResolution: "1080p",
  targetChannels: undefined,
  encoder: "libx264",
};

describe("streamLineParts", () => {
  it("shows a codec change with an arrow and the target resolution", () => {
    expect(streamLineParts(videoLine)).toEqual(["HEVC → H.264", "1080p"]);
  });

  it("shows a copied codec once, with the target channels", () => {
    const audioLine: StreamLine = {
      ...videoLine,
      kind: "audio",
      sourceCodec: "AAC",
      targetCodec: "AAC",
      targetResolution: undefined,
      targetChannels: "5.1",
    };

    expect(streamLineParts(audioLine)).toEqual(["AAC", "5.1"]);
  });
});
