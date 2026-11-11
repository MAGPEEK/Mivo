import { describe, expect, it } from "vitest";

import { imageSubtitle, subtitle } from "./test/tracks";
import {
  audioTracks,
  sessionAudioIndex,
  sessionSidecarTrack,
  sessionSubtitle,
  subtitleTracks,
  type TrackSession,
  type TrackSource,
  type TrackSourceSubtitle,
} from "./trackModel";

const textTrack: TrackSourceSubtitle = {
  id: "s:3",
  language: "ger",
  format: "subrip",
  forced: false,
  sdh: false,
  default: true,
  external: false,
  textBased: true,
  url: "/api/v1/media-files/f/subtitles/s:3.vtt",
};

const file: TrackSource = {
  streams: [
    { index: 0, kind: "video", codec: "h264", default: true },
    { index: 1, kind: "audio", codec: "ac3", channels: 6, language: "ger", default: true },
    {
      index: 2,
      kind: "audio",
      codec: "aac",
      channels: 2,
      title: "Director's Commentary",
      default: false,
    },
    { index: 3, kind: "subtitle", codec: "subrip", default: false },
    { index: 4, kind: "hologram", default: false },
  ],
  subtitleTracks: [
    textTrack,
    { ...textTrack, id: "s:5", format: "hdmv_pgs_subtitle", textBased: false, url: undefined },
    { ...textTrack, id: "x:ab12", external: true, url: "" },
  ],
};

function session(fields: Partial<TrackSession> = {}): TrackSession {
  return { streams: [], ...fields };
}

describe("audioTracks", () => {
  it("lists the audio streams in container order", () => {
    expect(audioTracks(file)).toEqual([
      {
        index: 1,
        codec: "ac3",
        channels: 6,
        language: "ger",
        title: undefined,
        default: true,
        commentary: false,
      },
      {
        index: 2,
        codec: "aac",
        channels: 2,
        language: undefined,
        title: "Director's Commentary",
        default: false,
        commentary: true,
      },
    ]);
  });

  it.each<[string | undefined, boolean]>([
    ["Kommentar des Regisseurs", true],
    ["Audio Description", true],
    ["Descriptive Audio", true],
    ["Audiodeskription", true],
    ["Hörfilm", true],
    ["Deutsch 5.1", false],
    [undefined, false],
  ])("title %s marks commentary: %s", (title, expected) => {
    const [track] = audioTracks({ streams: [{ index: 1, kind: "audio", title, default: false }] });
    expect(track?.commentary).toBe(expected);
  });

  it("is empty for a file without audio", () => {
    expect(audioTracks({ streams: [] })).toEqual([]);
  });
});

describe("subtitleTracks", () => {
  it("marks tracks with a WebVTT rendition as sidecar", () => {
    expect(subtitleTracks(file).map(({ id, sidecar }) => [id, sidecar])).toEqual([
      ["s:3", true],
      ["s:5", false],
      ["x:ab12", false],
    ]);
  });

  it("is empty when the server lists no subtitle tracks", () => {
    expect(subtitleTracks({ streams: file.streams })).toEqual([]);
  });
});

describe("sessionAudioIndex", () => {
  it("returns the source index of the audio stream", () => {
    const playing = session({
      streams: [
        { kind: "video", sourceIndex: 0, action: "copy", sourceCodec: "h264", targetCodec: "h264" },
        { kind: "audio", sourceIndex: 2, action: "copy", sourceCodec: "aac", targetCodec: "aac" },
      ],
    });
    expect(sessionAudioIndex(playing)).toBe(2);
  });

  it("is undefined for a session without audio", () => {
    expect(sessionAudioIndex(session())).toBeUndefined();
  });
});

describe("sessionSubtitle", () => {
  it("is off for a session without a subtitle decision", () => {
    expect(sessionSubtitle(session())).toEqual({ mode: "off" });
  });

  it("returns the decision of the session", () => {
    const subtitleDecision = { mode: "burn_in", trackId: "s:5" } as const;
    expect(sessionSubtitle(session({ subtitle: subtitleDecision }))).toBe(subtitleDecision);
  });
});

describe("sessionSidecarTrack", () => {
  const tracks = [subtitle("s:3"), imageSubtitle("s:5")];

  it.each<{ name: string; subtitle: TrackSession["subtitle"]; expected: string | undefined }>([
    { name: "a sidecar track", subtitle: { mode: "sidecar", trackId: "s:3" }, expected: "s:3" },
    { name: "no decision", subtitle: undefined, expected: undefined },
    { name: "off", subtitle: { mode: "off" }, expected: undefined },
    { name: "burn-in", subtitle: { mode: "burn_in", trackId: "s:5" }, expected: undefined },
    { name: "a sidecar without id", subtitle: { mode: "sidecar" }, expected: undefined },
    {
      name: "an unknown track",
      subtitle: { mode: "sidecar", trackId: "s:9" },
      expected: undefined,
    },
    {
      name: "a track without rendition",
      subtitle: { mode: "sidecar", trackId: "s:5" },
      expected: undefined,
    },
  ])("$name", ({ subtitle: decision, expected }) => {
    const playing = decision === undefined ? session() : session({ subtitle: decision });
    expect(sessionSidecarTrack(playing, tracks)?.id).toBe(expected);
  });
});
