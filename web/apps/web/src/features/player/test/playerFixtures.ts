// Fixtures of the player feature: client profiles, an item with one file and sessions shaped
// like the server's answers (ADR 0024).

import type { ClientProfile, ItemDetail, PlaybackSession } from "@mivo/api-client";

/** ID of the item every player test opens. */
export const playerItemId = "item-1";

/** ID of the item's only file. */
export const playerFileId = "file-1";

/** Probed duration of the item's file in seconds. */
export const playerFileDurationSeconds = 120;

const profileCodecs = { videoCodecs: ["h264", "vp9"], audioCodecs: ["aac", "opus"] };

/** A desktop browser that plays HLS through hls.js (Media Source Extensions). */
export const mseProfile: ClientProfile = {
  name: "Test browser",
  directPlay: [{ container: "mp4", ...profileCodecs }],
  hls: { delivery: "mse", ...profileCodecs },
};

/** A browser that plays HLS with the media element itself (iOS Safari). */
export const nativeHlsProfile: ClientProfile = {
  ...mseProfile,
  hls: { delivery: "native", ...profileCodecs },
};

/** A browser without any HLS support: only direct play is possible. */
export const noHlsProfile: ClientProfile = {
  ...mseProfile,
  hls: { delivery: "none", videoCodecs: [], audioCodecs: [] },
};

/** The item the player opens, with one playable file. */
export function playerItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id: playerItemId,
    libraryId: "library-1",
    title: "Big Buck Bunny",
    files: [
      {
        id: playerFileId,
        fileName: "Big.Buck.Bunny.mp4",
        sizeBytes: 1_000_000,
        modifiedAt: "2026-10-01T09:30:00Z",
        available: true,
        container: "mp4",
        durationSeconds: playerFileDurationSeconds,
        streams: [],
      },
    ],
    ...overrides,
  };
}

/** Fields of a fake session a test usually varies. */
export interface SessionFields {
  readonly id: string;
  readonly delivery?: PlaybackSession["delivery"];
  readonly startPositionSeconds?: number;
  readonly state?: PlaybackSession["state"];
}

/** A session the fake server returns: direct play unless `delivery` says `hls`. */
export function fakeSession(fields: SessionFields): PlaybackSession {
  const { id, delivery = "file", startPositionSeconds = 0, state = "ready" } = fields;
  const stream = delivery === "file" ? "file" : "hls/index.m3u8";
  return {
    id,
    fileId: playerFileId,
    state,
    mode: delivery === "file" ? "direct_play" : "remux",
    delivery,
    streamUrl: `/api/v1/playback/sessions/${id}/${stream}`,
    startPositionSeconds,
    durationSeconds: playerFileDurationSeconds,
    sourceContainer: "mp4",
    targetContainer: "mp4",
    streams: [],
    reasons: [],
    warnings: [],
  };
}

/** A direct-play session of an MP4 with H.264 and AAC (one session per mode for the overlay). */
export const directPlaySession: PlaybackSession = {
  ...fakeSession({ id: "direct-1", startPositionSeconds: 30 }),
  sourceBitRate: 950_000,
  streams: [
    { kind: "video", sourceIndex: 0, action: "copy", sourceCodec: "h264", targetCodec: "h264" },
    { kind: "audio", sourceIndex: 1, action: "copy", sourceCodec: "aac", targetCodec: "aac" },
  ],
};

/** A remux of an MKV whose container the browser does not play. */
export const remuxSession: PlaybackSession = {
  ...directPlaySession,
  id: "remux-1",
  mode: "remux",
  delivery: "hls",
  streamUrl: "/api/v1/playback/sessions/remux-1/hls/index.m3u8",
  sourceContainer: "mkv",
  sourceBitRate: 4_073_000,
  reasons: [{ code: "container_not_supported", scope: "container" }],
};

/** A transcode of a 4K HDR HEVC MKV with AC-3 5.1 audio and subtitles. */
export const transcodeSession: PlaybackSession = {
  ...remuxSession,
  id: "transcode-1",
  mode: "transcode",
  streamUrl: "/api/v1/playback/sessions/transcode-1/hls/index.m3u8",
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
