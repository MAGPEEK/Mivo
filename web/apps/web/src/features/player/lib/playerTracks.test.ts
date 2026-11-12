import { describe, expect, it } from "vitest";

import { multiTrackItem, playerFileId, playerItem } from "../test/playerFixtures";
import { playerTracks } from "./playerTracks";

describe("playerTracks", () => {
  it("lists the audio and subtitle tracks and the quality steps below the source", () => {
    const tracks = playerTracks(multiTrackItem(), playerFileId);

    expect(tracks.audio.map((track) => track.index)).toEqual([1, 2, 3]);
    expect(tracks.subtitles.map((track) => [track.id, track.sidecar])).toEqual([
      ["s:4", true],
      ["s:5", true],
      ["s:6", false],
      ["x:9f2c41", true],
    ]);
    // The 20 Mbit/s step lies above the 14.2 Mbit/s source and is left out.
    expect(tracks.quality.map((option) => option.maxBitRate)).toEqual([
      0, 12_000_000, 8_000_000, 4_000_000, 2_000_000, 1_000_000,
    ]);
  });

  it("offers every step for a source of unknown bit rate", () => {
    const tracks = playerTracks(playerItem(), playerFileId);

    expect(tracks.quality).toHaveLength(7);
    expect(tracks.audio).toEqual([]);
    expect(tracks.subtitles).toEqual([]);
  });

  it("offers no tracks for a file the item does not list", () => {
    const tracks = playerTracks(multiTrackItem(), "other-file");

    expect(tracks.audio).toEqual([]);
    expect(tracks.subtitles).toEqual([]);
    expect(tracks.quality[0]?.kind).toBe("original");
  });
});
