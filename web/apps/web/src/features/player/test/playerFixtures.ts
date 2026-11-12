// Fixtures of the player feature: client profiles, an item with one file and sessions shaped
// like the server's answers (ADR 0024).

import type { ClientProfile, ItemDetail, MediaStream, PlaybackSession } from "@mivo/api-client";

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
    kind: "movie",
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

/** A stream of a probed file; a test names only what differs from the defaults. */
function stream(fields: Pick<MediaStream, "index" | "kind"> & Partial<MediaStream>): MediaStream {
  return { default: false, forced: false, attachedPicture: false, ...fields };
}

/** ID of the English text subtitle of {@link multiTrackItem}. */
export const englishSubtitleId = "s:5";
/** ID of the image subtitle (PGS) of {@link multiTrackItem}, which needs burn-in. */
export const imageSubtitleId = "s:6";
/** ID of the external ASS subtitle of {@link multiTrackItem}. */
export const externalSubtitleId = "x:9f2c41";

/**
 * An item whose file has a 14.2 Mbit/s H.264 video, three audio tracks (German E-AC-3 5.1,
 * English AAC, English AAC commentary) and four subtitles: forced German text, English SDH
 * text, an English image track (PGS) and an external French ASS track.
 */
export function multiTrackItem(): ItemDetail {
  const item = playerItem();
  const [file] = item.files;
  if (file === undefined) {
    throw new Error("the player item has no file");
  }
  return {
    ...item,
    files: [
      {
        ...file,
        container: "mkv",
        bitRate: 14_200_000,
        height: 1080,
        streams: [
          stream({ index: 0, kind: "video", codec: "h264", height: 1080, default: true }),
          stream({
            index: 1,
            kind: "audio",
            codec: "eac3",
            channels: 6,
            language: "ger",
            title: "Deutsch 5.1",
            default: true,
          }),
          stream({ index: 2, kind: "audio", codec: "aac", channels: 2, language: "eng" }),
          stream({
            index: 3,
            kind: "audio",
            codec: "aac",
            channels: 2,
            language: "eng",
            title: "Director's commentary",
          }),
        ],
        subtitleTracks: [
          {
            id: "s:4",
            language: "ger",
            title: "Deutsch (Forced)",
            format: "subrip",
            forced: true,
            sdh: false,
            default: true,
            external: false,
            textBased: true,
            url: "/api/v1/media-files/file-1/subtitles/s:4.vtt",
          },
          {
            id: englishSubtitleId,
            language: "eng",
            format: "subrip",
            forced: false,
            sdh: true,
            default: false,
            external: false,
            textBased: true,
            url: "/api/v1/media-files/file-1/subtitles/s:5.vtt",
          },
          {
            id: imageSubtitleId,
            language: "eng",
            format: "hdmv_pgs_subtitle",
            forced: false,
            sdh: false,
            default: false,
            external: false,
            textBased: false,
          },
          {
            id: externalSubtitleId,
            language: "fra",
            format: "ass",
            forced: false,
            sdh: false,
            default: false,
            external: true,
            textBased: true,
            url: "/api/v1/media-files/file-1/subtitles/x:9f2c41.vtt",
          },
        ],
      },
    ],
  };
}

/** A transcode of {@link multiTrackItem}'s file: German E-AC-3 converted to AAC, no subtitle. */
export const multiTrackSession: PlaybackSession = {
  ...transcodeSession,
  id: "multi-1",
  fileId: playerFileId,
  sourceBitRate: 14_200_000,
  streams: [
    {
      kind: "video",
      sourceIndex: 0,
      action: "copy",
      sourceCodec: "h264",
      targetCodec: "h264",
    },
    {
      kind: "audio",
      sourceIndex: 1,
      action: "transcode",
      sourceCodec: "eac3",
      targetCodec: "aac",
      targetChannels: 2,
      encoder: "aac",
    },
  ],
  reasons: [{ code: "audio_codec_not_supported", scope: "audio" }],
  warnings: [],
  subtitle: { mode: "off" },
};
