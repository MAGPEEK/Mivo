import { describe, expect, it } from "vitest";

import type { ClientProfile } from "../profile/clientProfile";
import {
  defaultAudioTrack,
  defaultSubtitleTrack,
  trackPreferenceOf,
  type TrackPreference,
} from "./defaultTracks";
import { audio, imageSubtitle, subtitle } from "./test/tracks";
import type { AudioTrack, SubtitleTrack } from "./trackModel";

describe("defaultAudioTrack", () => {
  it.each<{
    name: string;
    tracks: AudioTrack[];
    preference?: TrackPreference;
    expected: number | undefined;
  }>([
    { name: "no audio", tracks: [], expected: undefined },
    { name: "file order without flags", tracks: [audio(1), audio(2)], expected: 1 },
    {
      name: "the default flag before file order",
      tracks: [audio(1), audio(2, { default: true })],
      expected: 2,
    },
    {
      name: "the preferred language before the default flag",
      tracks: [audio(1, { language: "eng", default: true }), audio(2, { language: "ger" })],
      preference: { audioLanguages: ["de"] },
      expected: 2,
    },
    {
      name: "the earliest preferred language",
      tracks: [
        audio(1, { language: "fra" }),
        audio(2, { language: "eng" }),
        audio(3, { language: "deu" }),
      ],
      preference: { audioLanguages: ["de", "en"] },
      expected: 3,
    },
    {
      name: "no commentary before the preferred language",
      tracks: [
        audio(1, { language: "ger", commentary: true, default: true }),
        audio(2, { language: "eng" }),
      ],
      preference: { audioLanguages: ["de", "en"] },
      expected: 2,
    },
    {
      name: "a commentary when there is nothing else",
      tracks: [audio(1, { commentary: true }), audio(2, { commentary: true, default: true })],
      expected: 2,
    },
    {
      name: "channels do not rank",
      tracks: [audio(1, { channels: 2 }), audio(2, { channels: 6 })],
      expected: 1,
    },
    {
      name: "an unmatched preference falls back to the default flag",
      tracks: [audio(1, { language: "eng" }), audio(2, { language: "fra", default: true })],
      preference: { audioLanguages: ["de"] },
      expected: 2,
    },
  ])("picks $name", ({ tracks, preference, expected }) => {
    expect(defaultAudioTrack(tracks, preference)?.index).toBe(expected);
  });
});

describe("defaultSubtitleTrack", () => {
  const forcedGerman = subtitle("s:3", { language: "ger", forced: true });
  const fullGerman = subtitle("s:4", { language: "ger" });
  const sdhGerman = subtitle("s:5", { language: "ger", sdh: true });
  const fullEnglish = subtitle("s:6", { language: "eng" });
  const all = [forcedGerman, fullGerman, sdhGerman, fullEnglish];

  it.each<{
    name: string;
    tracks: SubtitleTrack[];
    audioLanguage?: string;
    preference?: TrackPreference;
    expected: string | undefined;
  }>([
    {
      name: "off shows nothing",
      tracks: all,
      audioLanguage: "de",
      preference: { subtitleMode: "off" },
      expected: undefined,
    },
    {
      name: "an omitted mode shows the forced track of the audio language",
      tracks: all,
      audioLanguage: "de",
      expected: "s:3",
    },
    {
      name: "forced shows nothing for another audio language",
      tracks: all,
      audioLanguage: "en",
      preference: { subtitleMode: "forced" },
      expected: undefined,
    },
    {
      name: "forced shows nothing for an undetermined audio language",
      tracks: all,
      audioLanguage: "und",
      expected: undefined,
    },
    { name: "forced shows nothing without an audio language", tracks: all, expected: undefined },
    {
      name: "forced never burns in",
      tracks: [imageSubtitle("s:7", { language: "ger", forced: true })],
      audioLanguage: "de",
      expected: undefined,
    },
    {
      name: "always shows a full track in the first subtitle language",
      tracks: all,
      preference: {
        subtitleMode: "always",
        subtitleLanguages: ["en", "de"],
        audioLanguages: ["de"],
      },
      expected: "s:6",
    },
    {
      name: "always falls back to the audio languages",
      tracks: all,
      preference: { subtitleMode: "always", audioLanguages: ["de"] },
      expected: "s:4",
    },
    {
      name: "always prefers the SDH kind asked for",
      tracks: all,
      preference: { subtitleMode: "always", subtitleLanguages: ["de"], preferSdh: true },
      expected: "s:5",
    },
    {
      name: "always takes the first track without any language",
      tracks: all.slice(1),
      preference: { subtitleMode: "always" },
      expected: "s:4",
    },
    {
      name: "always shows nothing when no language matches",
      tracks: all,
      preference: { subtitleMode: "always", subtitleLanguages: ["fr"] },
      expected: undefined,
    },
    {
      name: "always ranks a full track of a later language above a forced-only one",
      tracks: [forcedGerman, fullEnglish],
      preference: { subtitleMode: "always", subtitleLanguages: ["de", "en"] },
      expected: "s:6",
    },
    {
      name: "always takes a forced track when no full one exists",
      tracks: [forcedGerman],
      preference: { subtitleMode: "always", subtitleLanguages: ["de"] },
      expected: "s:3",
    },
    {
      name: "always prefers the default flag among equals",
      tracks: [fullGerman, subtitle("s:8", { language: "ger", default: true })],
      preference: { subtitleMode: "always", subtitleLanguages: ["de"] },
      expected: "s:8",
    },
    {
      name: "always skips tracks that need burn-in",
      tracks: [imageSubtitle("s:7", { language: "ger" }), fullGerman],
      preference: { subtitleMode: "always", subtitleLanguages: ["de"] },
      expected: "s:4",
    },
    {
      name: "an unknown mode shows nothing",
      tracks: all,
      audioLanguage: "de",
      preference: { subtitleMode: "sometimes" as TrackPreference["subtitleMode"] },
      expected: undefined,
    },
  ])("$name", ({ tracks, audioLanguage, preference, expected }) => {
    expect(defaultSubtitleTrack(tracks, audioLanguage, preference)?.id).toBe(expected);
  });
});

describe("trackPreferenceOf", () => {
  it("reads the preference of a client profile", () => {
    const profile: ClientProfile = {
      name: "Test",
      directPlay: [],
      hls: { delivery: "mse", videoCodecs: [], audioCodecs: [] },
      preferredAudioLanguages: ["de"],
      preferredSubtitleLanguages: ["en"],
      subtitleMode: "always",
      preferSdh: true,
    };
    expect(trackPreferenceOf(profile)).toEqual({
      audioLanguages: ["de"],
      subtitleLanguages: ["en"],
      subtitleMode: "always",
      preferSdh: true,
    });
  });
});
