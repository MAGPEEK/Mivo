// Example sessions shaped like the server's answers (ADR 0024), one per mode.

import type { PlaybackSession, StreamDecision } from "../playbackSession";

const fileId = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f";

/** A direct-play session of an MP4 with H.264 and AAC. */
export const directPlaySession: PlaybackSession = {
  id: "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a",
  fileId,
  state: "ready",
  mode: "direct_play",
  delivery: "file",
  streamUrl: "/api/v1/playback/sessions/5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a/file",
  startPositionSeconds: 600,
  durationSeconds: 5423.042,
  sourceContainer: "mp4",
  targetContainer: "mp4",
  sourceBitRate: 4_073_000,
  streams: [
    copied({ kind: "video", sourceIndex: 0, codec: "h264" }),
    copied({ kind: "audio", sourceIndex: 1, codec: "aac" }),
  ],
  reasons: [],
  warnings: [],
};

/** A remux of an MKV whose container the browser does not play. */
export const remuxSession: PlaybackSession = {
  ...directPlaySession,
  id: "6e5d4c3b-2a1f-4e0d-9c8b-7a6f5e4d3c2b",
  state: "starting",
  mode: "remux",
  delivery: "hls",
  streamUrl: "/api/v1/playback/sessions/6e5d4c3b-2a1f-4e0d-9c8b-7a6f5e4d3c2b/hls/index.m3u8",
  startPositionSeconds: 598.5,
  sourceContainer: "mkv",
  reasons: [{ code: "container_not_supported", scope: "container" }],
};

/** A transcode of a 4K HDR HEVC MKV with AC-3 5.1 audio and subtitles. */
export const transcodeSession: PlaybackSession = {
  ...remuxSession,
  id: "7f6e5d4c-3b2a-4f1e-8d9c-8b7a6f5e4d3c",
  mode: "transcode",
  startPositionSeconds: 600,
  sourceBitRate: 18_500_000,
  streams: [
    {
      kind: "video",
      sourceIndex: 0,
      action: "transcode",
      sourceCodec: "hevc",
      targetCodec: "h264",
      targetWidth: 1920,
      targetHeight: 1080,
      encoder: "libx264",
    },
    {
      kind: "audio",
      sourceIndex: 2,
      action: "transcode",
      sourceCodec: "ac3",
      targetCodec: "aac",
      targetChannels: 2,
      encoder: "aac",
    },
  ],
  reasons: [
    { code: "container_not_supported", scope: "container" },
    { code: "video_codec_not_supported", scope: "video" },
    { code: "audio_codec_not_supported", scope: "audio" },
  ],
  warnings: [
    { code: "hdr_not_tone_mapped", scope: "video" },
    { code: "video_downscaled", scope: "video" },
    { code: "subtitles_not_supported", scope: "subtitle" },
  ],
};

interface CopiedStream {
  readonly kind: StreamDecision["kind"];
  readonly sourceIndex: number;
  readonly codec: string;
}

/** A stream decision that passes the stream through unchanged. */
export function copied({ kind, sourceIndex, codec }: CopiedStream): StreamDecision {
  return { kind, sourceIndex, action: "copy", sourceCodec: codec, targetCodec: codec };
}
