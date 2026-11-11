import { describe, expect, it } from "vitest";

import type { ClientProfile } from "./clientProfile";
import {
  renderedSubtitleFormats,
  withPlaybackPreferences,
  type PlaybackPreferences,
} from "./playbackPreferences";

const detected: ClientProfile = {
  name: "Test",
  directPlay: [{ container: "mp4", videoCodecs: ["h264"], audioCodecs: ["aac"] }],
  hls: { delivery: "mse", videoCodecs: ["h264"], audioCodecs: ["aac"] },
  codecLimits: [{ codec: "h264", maxBitDepth: 8 }],
};
const rendered = { ...detected, subtitleFormats: ["webvtt"] };

describe("withPlaybackPreferences", () => {
  it.each<{ name: string; preferences?: PlaybackPreferences; expected: ClientProfile }>([
    { name: "no preferences still announce WebVTT", expected: rendered },
    {
      name: "empty and absent settings are left out",
      preferences: { audioLanguages: [], subtitleLanguages: ["?"] },
      expected: rendered,
    },
    {
      name: "the track preference",
      preferences: {
        audioLanguages: ["de-DE", "de", "en"],
        subtitleLanguages: ["en"],
        subtitleMode: "always",
        preferSdh: false,
      },
      expected: {
        ...rendered,
        preferredAudioLanguages: ["de-DE", "en"],
        preferredSubtitleLanguages: ["en"],
        subtitleMode: "always",
        preferSdh: false,
      },
    },
    {
      name: "the audio limits",
      preferences: {
        audioLimits: { maxAudioChannels: 6, maxAudioSampleRate: 96_000, maxAudioBitDepth: 24 },
      },
      expected: {
        ...rendered,
        maxAudioChannels: 6,
        maxAudioSampleRate: 96_000,
        maxAudioBitDepth: 24,
      },
    },
  ])("$name", ({ preferences, expected }) => {
    expect(withPlaybackPreferences(detected, preferences)).toStrictEqual(expected);
  });

  it("does not modify the detected profile or the shared format list", () => {
    const profile = withPlaybackPreferences(detected, { audioLanguages: ["de"] });
    profile.subtitleFormats?.push("ass");
    expect(detected).not.toHaveProperty("subtitleFormats");
    expect(detected).not.toHaveProperty("preferredAudioLanguages");
    expect(renderedSubtitleFormats).toEqual(["webvtt"]);
  });
});
