import { describe, expect, it } from "vitest";

import {
  imageSubtitleId,
  multiTrackItem,
  multiTrackSession,
  playerFileId,
} from "../test/playerFixtures";
import {
  activeStep,
  audioRows,
  currentAudioIndex,
  qualityRows,
  subtitleIsBurnedIn,
  subtitleRows,
  visibleQualityRows,
} from "./optionRows";
import { playerTracks } from "./playerTracks";

const tracks = playerTracks(multiTrackItem(), playerFileId);
const hlsAudioCodecs = ["aac", "opus"];

describe("currentAudioIndex", () => {
  it("prefers the viewer's choice over the session's", () => {
    expect(currentAudioIndex({ audioStreamIndex: 2 }, multiTrackSession)).toBe(2);
    expect(currentAudioIndex({}, multiTrackSession)).toBe(1);
    expect(currentAudioIndex({}, undefined)).toBeUndefined();
  });
});

describe("audioRows", () => {
  it("keeps file order, marks the playing track and the tracks the server converts", () => {
    const rows = audioRows(tracks.audio, 1, hlsAudioCodecs);

    expect(
      rows.map((row) => [row.index, row.language, row.details, row.converted, row.current]),
    ).toEqual([
      [1, "ger", "E-AC-3 · 5.1", true, true],
      [2, "eng", "AAC · 2.0", false, false],
      [3, "eng", "AAC · 2.0", false, false],
    ]);
    expect(rows[2]?.title).toBe("Director's commentary");
  });
});

describe("subtitleRows", () => {
  it("labels formats and flags burn-in, unstyled text and a refused burn-in", () => {
    const rows = subtitleRows(tracks.subtitles, "s:5", imageSubtitleId);

    expect(rows.map((row) => [row.id, row.format, row.burnIn, row.unstyled, row.current])).toEqual([
      ["s:4", "SRT", false, false, false],
      ["s:5", "SRT", false, false, true],
      ["s:6", "PGS", true, false, false],
      ["x:9f2c41", "ASS", false, true, false],
    ]);
    expect(rows.filter((row) => row.refused).map((row) => row.id)).toEqual([imageSubtitleId]);
    expect(rows[0]).toMatchObject({ forced: true, sdh: false, external: false });
    expect(rows[1]).toMatchObject({ forced: false, sdh: true });
    expect(rows[3]).toMatchObject({ external: true });
  });
});

describe("subtitleIsBurnedIn", () => {
  it("is true only while the session burns a subtitle in", () => {
    expect(subtitleIsBurnedIn(undefined)).toBe(false);
    expect(subtitleIsBurnedIn(multiTrackSession)).toBe(false);
    expect(
      subtitleIsBurnedIn({ ...multiTrackSession, subtitle: { mode: "burn_in", trackId: "s:6" } }),
    ).toBe(true);
  });
});

describe("qualityRows", () => {
  const original = { kind: "original" } as const;

  it("lists the original, Automatic and the steps below the source, best first", () => {
    const rows = qualityRows(tracks.quality, original, true);

    expect(rows.map((row) => row.kind)).toEqual([
      "original",
      "automatic",
      "limited",
      "limited",
      "limited",
      "limited",
      "limited",
    ]);
    expect(rows.filter((row) => row.current).map((row) => row.kind)).toEqual(["original"]);
    expect(rows[2]).toMatchObject({ megabits: 12, height: 1080 });
    expect(rows[5]).toMatchObject({ megabits: 2, height: 480 });
  });

  it("leaves Automatic out when the browser cannot measure the speed", () => {
    expect(qualityRows(tracks.quality, original, false).map((row) => row.kind)).not.toContain(
      "automatic",
    );
  });

  it("marks Automatic and a chosen step", () => {
    expect(
      qualityRows(tracks.quality, { kind: "automatic" }, true).find((row) => row.current)?.kind,
    ).toBe("automatic");
    const step = qualityRows(tracks.quality, { kind: "limited", maxBitRate: 4_000_000 }, true).find(
      (row) => row.current,
    );
    expect(step).toMatchObject({ kind: "limited", megabits: 4 });
  });

  it("hides the steps until asked for, but keeps a chosen one", () => {
    const rows = qualityRows(tracks.quality, { kind: "limited", maxBitRate: 4_000_000 }, true);

    expect(visibleQualityRows(rows, false).map((row) => row.megabits)).toEqual([
      undefined,
      undefined,
      4,
    ]);
    expect(visibleQualityRows(rows, true)).toHaveLength(rows.length);
  });

  it("names the step Automatic plays at, none for the original", () => {
    const rows = qualityRows(tracks.quality, { kind: "automatic" }, true);

    expect(activeStep(rows, { maxBitRate: 8_000_000 })).toMatchObject({ megabits: 8 });
    expect(activeStep(rows, { maxBitRate: 0 })).toBeUndefined();
    expect(activeStep(rows, {})).toBeUndefined();
  });
});
