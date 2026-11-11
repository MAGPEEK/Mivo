// Audio limits of the client profile (ADR 0030 Notes M4-02b): how many channels, which sample
// rate and which sample size the client plays. They are probed like codecs, through Media
// Capabilities, and never guessed from the browser's name. A limit that cannot be measured is
// left out, which means the server's default.

import type { ClientProfile } from "./clientProfile";
import { timedDecodingInfo } from "./decodingCheck";
import type { AudioDecodingConfiguration, ClientProbes } from "./probes";

/** The audio limits of a client profile; an omitted limit means the server's default. */
export type AudioLimits = Pick<
  ClientProfile,
  "maxAudioChannels" | "maxAudioSampleRate" | "maxAudioBitDepth"
>;

/** Bounds of the API schema `ClientProfile` for the three limits. */
export const audioLimitBounds = {
  channels: { minimum: 1, maximum: 32 },
  sampleRate: { minimum: 8000, maximum: 768_000 },
  bitDepth: { minimum: 8, maximum: 64 },
} as const;

/** Channel counts asked for, most first: 7.1, 5.1, stereo. */
export const probedChannelCounts: readonly number[] = [8, 6, 2];

/** Sample rates asked for in Hz, highest first. */
export const probedSampleRates: readonly number[] = [192_000, 96_000, 48_000];

// Channels are asked for AAC in MP4, the codec the server re-encodes audio to, so the answer
// says whether a 5.1 transcode plays. Sample rates are asked for FLAC, the codec hi-res files
// come in.
const channelProbeContentType = 'audio/mp4; codecs="mp4a.40.2"';
const sampleRateProbeContentType = "audio/flac";

/** What the caller knows about the audio output beyond what the browser APIs can be asked. */
export interface AudioLimitHints {
  /**
   * Highest sample size the device plays, in bits. No browser API reports it, so only a
   * platform that knows its output (the TV app) passes it; the web app leaves it out.
   */
  readonly maxAudioBitDepth?: number | undefined;
}

interface Bounds {
  readonly minimum: number;
  readonly maximum: number;
}

/** The value as an integer inside the bounds; `undefined` when it is no number at all. */
function bounded(value: number | undefined, bounds: Bounds): number | undefined {
  if (value === undefined || !Number.isFinite(value)) {
    return undefined;
  }
  return Math.min(Math.max(Math.trunc(value), bounds.minimum), bounds.maximum);
}

type AudioLimitProbes = Pick<ClientProbes, "mediaCapabilities" | "startTimer">;

/** The first candidate the client says it decodes; `undefined` when it accepts none. */
async function highestDecoded(
  probes: AudioLimitProbes,
  candidates: readonly number[],
  configurationOf: (candidate: number) => AudioDecodingConfiguration,
): Promise<number | undefined> {
  const { mediaCapabilities, startTimer } = probes;
  if (mediaCapabilities === undefined) {
    return undefined;
  }
  const answers = await Promise.all(
    candidates.map((candidate) =>
      timedDecodingInfo(
        mediaCapabilities,
        { type: "file", audio: configurationOf(candidate) },
        startTimer,
      ),
    ),
  );
  return candidates.find((_, position) => answers[position] === true);
}

/**
 * Detects the audio limits of this client. Channels and sample rate are the highest of
 * {@link probedChannelCounts} and {@link probedSampleRates} that Media Capabilities reports as
 * decodable; without that API, or when it accepts no candidate, the limit is omitted. The
 * sample size comes from the hints only. Every value is bounded by {@link audioLimitBounds}.
 */
export async function detectAudioLimits(
  probes: AudioLimitProbes,
  hints: AudioLimitHints = {},
): Promise<AudioLimits> {
  const [channels, sampleRate] = await Promise.all([
    highestDecoded(probes, probedChannelCounts, (count) => ({
      contentType: channelProbeContentType,
      channels: String(count),
    })),
    highestDecoded(probes, probedSampleRates, (rate) => ({
      contentType: sampleRateProbeContentType,
      samplerate: rate,
    })),
  ]);
  return withoutUndefined({
    maxAudioChannels: bounded(channels, audioLimitBounds.channels),
    maxAudioSampleRate: bounded(sampleRate, audioLimitBounds.sampleRate),
    maxAudioBitDepth: bounded(hints.maxAudioBitDepth, audioLimitBounds.bitDepth),
  });
}

/** Drops absent limits, so the profile serialises without `undefined` members. */
function withoutUndefined(limits: {
  readonly [Key in keyof AudioLimits]: number | undefined;
}): AudioLimits {
  const result: AudioLimits = {};
  if (limits.maxAudioChannels !== undefined) {
    result.maxAudioChannels = limits.maxAudioChannels;
  }
  if (limits.maxAudioSampleRate !== undefined) {
    result.maxAudioSampleRate = limits.maxAudioSampleRate;
  }
  if (limits.maxAudioBitDepth !== undefined) {
    result.maxAudioBitDepth = limits.maxAudioBitDepth;
  }
  return result;
}
