import { describe, expect, it } from "vitest";

import { audioLimitBounds, detectAudioLimits, type AudioLimits } from "./audioLimits";
import type { AudioDecodingConfiguration, DecodingConfiguration, StartTimer } from "./probes";

const neverFires: StartTimer = () => () => undefined;

/** Media Capabilities that decode audio up to the given channel count and sample rate. */
function capabilities(maxChannels: number, maxSampleRate: number) {
  const queries: DecodingConfiguration[] = [];
  const supports = (audio: AudioDecodingConfiguration | undefined): boolean =>
    audio !== undefined &&
    Number(audio.channels ?? "2") <= maxChannels &&
    (audio.samplerate ?? 0) <= maxSampleRate;
  return {
    queries,
    decodingInfo(configuration: DecodingConfiguration) {
      queries.push(configuration);
      return Promise.resolve({ supported: supports(configuration.audio) });
    },
  };
}

describe("detectAudioLimits", () => {
  it.each<{ name: string; channels: number; sampleRate: number; expected: AudioLimits }>([
    {
      name: "a client that decodes everything",
      channels: 32,
      sampleRate: 768_000,
      expected: { maxAudioChannels: 8, maxAudioSampleRate: 192_000 },
    },
    {
      name: "5.1 at 96 kHz",
      channels: 6,
      sampleRate: 96_000,
      expected: { maxAudioChannels: 6, maxAudioSampleRate: 96_000 },
    },
    {
      name: "stereo at 48 kHz",
      channels: 2,
      sampleRate: 48_000,
      expected: { maxAudioChannels: 2, maxAudioSampleRate: 48_000 },
    },
    {
      name: "no sample rate candidate decodes",
      channels: 2,
      sampleRate: 44_100,
      expected: { maxAudioChannels: 2 },
    },
    { name: "nothing decodes", channels: 0, sampleRate: 0, expected: {} },
  ])("$name", async ({ channels, sampleRate, expected }) => {
    const mediaCapabilities = capabilities(channels, sampleRate);
    const limits = await detectAudioLimits({ mediaCapabilities, startTimer: neverFires });
    expect(limits).toStrictEqual(expected);
  });

  it("asks for AAC channels and FLAC sample rates as files", async () => {
    const mediaCapabilities = capabilities(8, 192_000);
    await detectAudioLimits({ mediaCapabilities, startTimer: neverFires });
    expect(mediaCapabilities.queries).toContainEqual({
      type: "file",
      audio: { contentType: 'audio/mp4; codecs="mp4a.40.2"', channels: "6" },
    });
    expect(mediaCapabilities.queries).toContainEqual({
      type: "file",
      audio: { contentType: "audio/flac", samplerate: 96_000 },
    });
  });

  it("omits the limits without Media Capabilities", async () => {
    expect(await detectAudioLimits({ startTimer: neverFires })).toStrictEqual({});
  });

  it("counts a rejected and a late answer as unsupported", async () => {
    const timers: (() => void)[] = [];
    const limits = detectAudioLimits({
      mediaCapabilities: {
        decodingInfo: ({ audio }) =>
          audio?.samplerate === undefined
            ? Promise.reject(new TypeError("unknown type"))
            : new Promise(() => undefined),
      },
      startTimer: (_milliseconds, onTimeout) => {
        timers.push(onTimeout);
        return () => undefined;
      },
    });
    for (const fire of timers) {
      fire();
    }
    expect(await limits).toStrictEqual({});
  });

  it.each<{ name: string; hint: number; expected: AudioLimits }>([
    { name: "a known sample size", hint: 24, expected: { maxAudioBitDepth: 24 } },
    { name: "a fractional sample size", hint: 24.9, expected: { maxAudioBitDepth: 24 } },
    {
      name: "a sample size below the bounds",
      hint: 1,
      expected: { maxAudioBitDepth: audioLimitBounds.bitDepth.minimum },
    },
    {
      name: "a sample size above the bounds",
      hint: 1000,
      expected: { maxAudioBitDepth: audioLimitBounds.bitDepth.maximum },
    },
    { name: "no number", hint: Number.NaN, expected: {} },
  ])("takes the bit depth from the hint: $name", async ({ hint, expected }) => {
    const limits = await detectAudioLimits({ startTimer: neverFires }, { maxAudioBitDepth: hint });
    expect(limits).toStrictEqual(expected);
  });
});
