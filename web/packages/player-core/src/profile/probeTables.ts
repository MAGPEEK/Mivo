// Probe tables of ADR 0023: which MIME types and RFC 6381 codec strings a client is asked
// about. They are plain data so the TV app (milestone 10) can reuse them with its own probes.

/**
 * Video codecs a client is probed for, as ffprobe names them. The order is the order of the
 * profile's codec limits, aligned with the Go reference profiles (h264, vp9, av1).
 */
export const videoCodecIds = ["h264", "hevc", "vp9", "av1", "vp8", "theora"] as const;
export type VideoCodecId = (typeof videoCodecIds)[number];

/** Audio codecs a client is probed for, as ffprobe names them. */
export const audioCodecIds = ["aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3"] as const;
export type AudioCodecId = (typeof audioCodecIds)[number];

/** One representative encoding of a codec, used as the `codecs` parameter of a probe. */
export interface CodecVariant {
  /** RFC 6381 codec string, e.g. `avc1.640028`. */
  readonly codecString: string;
  /** Bit depth of this variant; codecs whose variants carry it get a `maxBitDepth` limit. */
  readonly bitDepth?: number;
  /** Profile name as ffprobe reports it; codecs whose variants carry it get a `profiles` limit. */
  readonly profile?: string;
}

/**
 * Variants per video codec. A codec counts as playable in a context when any variant plays;
 * the variants that play anywhere define the codec limits. H.264 and HEVC list profiles because
 * browsers decode only some of them; AV1 and VP9 are limited by bit depth alone.
 */
export const videoCodecVariants: Readonly<Record<VideoCodecId, readonly CodecVariant[]>> = {
  h264: [
    { codecString: "avc1.42E01E", bitDepth: 8, profile: "Constrained Baseline" },
    { codecString: "avc1.42001E", bitDepth: 8, profile: "Baseline" },
    { codecString: "avc1.4D401E", bitDepth: 8, profile: "Main" },
    { codecString: "avc1.640028", bitDepth: 8, profile: "High" },
    { codecString: "avc1.6E0028", bitDepth: 10, profile: "High 10" },
  ],
  hevc: [
    { codecString: "hvc1.1.6.L120.B0", bitDepth: 8, profile: "Main" },
    { codecString: "hvc1.2.4.L120.B0", bitDepth: 10, profile: "Main 10" },
  ],
  av1: [
    { codecString: "av01.0.08M.08", bitDepth: 8 },
    { codecString: "av01.0.08M.10", bitDepth: 10 },
  ],
  vp9: [
    { codecString: "vp09.00.40.08", bitDepth: 8 },
    { codecString: "vp09.02.40.10", bitDepth: 10 },
  ],
  vp8: [{ codecString: "vp8" }],
  theora: [{ codecString: "theora" }],
};

/** Variants per audio codec; audio has no limits in milestone 2, so each has one string. */
export const audioCodecVariants: Readonly<Record<AudioCodecId, readonly CodecVariant[]>> = {
  aac: [{ codecString: "mp4a.40.2" }],
  mp3: [{ codecString: "mp4a.6B" }],
  opus: [{ codecString: "opus" }],
  vorbis: [{ codecString: "vorbis" }],
  flac: [{ codecString: "flac" }],
  ac3: [{ codecString: "ac-3" }],
  eac3: [{ codecString: "ec-3" }],
};

/** Where codecs are probed: MIME types plus the codecs that this context can carry. */
export interface ProbeContext {
  /** MIME type for video codec probes; for containers also the served `Content-Type`. */
  readonly videoMimeType: string;
  /**
   * MIME type for audio codec probes. Media Capabilities rejects a `video/*` type in an audio
   * query, so audio is always asked about with an audio MIME type.
   */
  readonly audioMimeType: string;
  readonly videoCodecs: readonly VideoCodecId[];
  readonly audioCodecs: readonly AudioCodecId[];
}

/** A direct-play container key of ADR 0023 with its probe context. */
export interface ContainerProbe extends ProbeContext {
  readonly container: string;
}

/**
 * Direct-play containers in ADR 0023 order. A container counts only when the media element
 * plays its bare video MIME type and at least one of its video codecs.
 */
export const containerProbes: readonly ContainerProbe[] = [
  {
    container: "mp4",
    videoMimeType: "video/mp4",
    audioMimeType: "audio/mp4",
    videoCodecs: ["h264", "hevc", "av1", "vp9"],
    audioCodecs: ["aac", "mp3", "opus", "flac", "ac3", "eac3"],
  },
  {
    container: "mov",
    videoMimeType: "video/quicktime",
    // QuickTime has no registered audio MIME type; it shares the ISO BMFF audio decoders.
    audioMimeType: "audio/mp4",
    videoCodecs: ["h264", "hevc"],
    audioCodecs: ["aac", "mp3", "ac3", "eac3"],
  },
  {
    container: "webm",
    videoMimeType: "video/webm",
    audioMimeType: "audio/webm",
    videoCodecs: ["vp8", "vp9", "av1"],
    audioCodecs: ["opus", "vorbis"],
  },
  {
    container: "mkv",
    videoMimeType: "video/x-matroska",
    audioMimeType: "audio/x-matroska",
    videoCodecs: ["h264", "hevc", "av1", "vp9", "vp8", "theora"],
    audioCodecs: ["aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3"],
  },
  {
    container: "ogg",
    videoMimeType: "video/ogg",
    audioMimeType: "audio/ogg",
    videoCodecs: ["theora"],
    audioCodecs: ["vorbis", "opus", "flac"],
  },
  {
    container: "mpegts",
    videoMimeType: "video/mp2t",
    audioMimeType: "audio/mp2t",
    videoCodecs: ["h264", "hevc"],
    audioCodecs: ["aac", "mp3", "ac3", "eac3"],
  },
  {
    container: "3gp",
    videoMimeType: "video/3gpp",
    audioMimeType: "audio/3gpp",
    videoCodecs: ["h264"],
    audioCodecs: ["aac"],
  },
];

/** MIME type of an HLS playlist; a media element that plays it delivers HLS natively. */
export const hlsPlaylistMimeType = "application/vnd.apple.mpegurl";

/** Codecs the server may copy into fMP4 HLS segments, probed for HLS delivery. */
export const hlsSegmentProbe: ProbeContext = {
  videoMimeType: "video/mp4",
  audioMimeType: "audio/mp4",
  videoCodecs: ["h264", "hevc", "av1", "vp9"],
  audioCodecs: ["aac", "mp3", "opus", "flac", "ac3", "eac3"],
};
