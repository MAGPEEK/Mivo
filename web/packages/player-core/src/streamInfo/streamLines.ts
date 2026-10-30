// Per-stream lines of the stream-info overlay ("Video HEVC → H.264 1080p, transcode") and the
// encoder line, formatted with the shared technical labels.

import { codecLabel } from "../labels/formatLabels";
import { channelLabel, resolutionLabel } from "../labels/streamLabels";
import type { StreamDecision } from "./playbackSession";
import {
  keyLookup,
  knownStreamActions,
  knownStreamKinds,
  type EncoderKey,
  type StreamActionKey,
  type StreamKindKey,
} from "./translationKeys";

/** One selected source stream and what the session does with it. */
export interface StreamLine {
  /** Kind as the server sent it, e.g. `video`. */
  readonly kind: string;
  readonly kindKey: StreamKindKey;
  /** Index of the source stream in its container, e.g. for a stable list key. */
  readonly sourceIndex: number;
  /** Action as the server sent it, e.g. `copy`. */
  readonly action: string;
  readonly actionKey: StreamActionKey;
  /** Label of the source codec, e.g. `HEVC`. */
  readonly sourceCodec: string;
  /** Label of the codec the client receives, e.g. `H.264`. */
  readonly targetCodec: string;
  /** Label of the video resolution the client receives, e.g. `1080p`; absent when unknown. */
  readonly targetResolution: string | undefined;
  /** Label of the audio channels the client receives, e.g. `2.0`; absent when unknown. */
  readonly targetChannels: string | undefined;
  /** FFmpeg encoder of a transcoded stream, e.g. `libx264`; absent when copied. */
  readonly encoder: string | undefined;
}

/** The encoder line, e.g. "Software (libx264)". */
export interface EncoderText {
  readonly key: EncoderKey;
  readonly params: { readonly encoder: string };
}

const streamKindKey = keyLookup("streamKind", knownStreamKinds);
const streamActionKey = keyLookup("action", knownStreamActions);

// FFmpeg names hardware encoders after the codec plus the acceleration API, e.g. h264_nvenc or
// hevc_vaapi. Milestone 2 only has libx264, but a hardware encoder must never read "software".
const hardwareEncoderSuffixes: readonly string[] = [
  "_amf",
  "_nvenc",
  "_qsv",
  "_rkmpp",
  "_v4l2m2m",
  "_vaapi",
  "_videotoolbox",
  "_vulkan",
];

/** The overlay line of one stream decision. */
export function streamLine(decision: StreamDecision): StreamLine {
  return {
    kind: decision.kind,
    kindKey: streamKindKey(decision.kind),
    sourceIndex: decision.sourceIndex,
    action: decision.action,
    actionKey: streamActionKey(decision.action),
    sourceCodec: codecLabel(decision.sourceCodec),
    targetCodec: codecLabel(decision.targetCodec),
    targetResolution: targetResolution(decision),
    targetChannels:
      decision.targetChannels === undefined ? undefined : channelLabel(decision.targetChannels),
    encoder: decision.encoder,
  };
}

/** Label of the target resolution, or `undefined` when the height is missing or unusable. */
export function targetResolution(decision: StreamDecision): string | undefined {
  if (decision.targetHeight === undefined) {
    return undefined;
  }
  const label = resolutionLabel(decision.targetHeight, decision.targetWidth);
  return label === "" ? undefined : label;
}

/**
 * The encoder of the first re-encoded stream (video comes first), or `undefined` when every
 * stream is copied.
 */
export function encoderText(decisions: readonly StreamDecision[]): EncoderText | undefined {
  const encoder = decisions.find(
    (decision) => decision.action === "transcode" && decision.encoder !== undefined,
  )?.encoder;
  if (encoder === undefined) {
    return undefined;
  }
  const hardware = hardwareEncoderSuffixes.some((suffix) => encoder.endsWith(suffix));
  return {
    key: hardware ? "streamInfo.encoder.hardware" : "streamInfo.encoder.software",
    params: { encoder },
  };
}
