// What a client plays (ADR 0023). These types are structurally identical to the API schema
// `ClientProfile` in api/openapi.yaml, written out here because player-core must not depend on
// the generated client. Arrays stay mutable on purpose: a readonly array would not be
// assignable to the generated API type, which the web app checks at compile time.

/** Lower-case identifier of a container (direct-play key) or a codec (ffprobe codec_name). */
export type MediaIdentifier = string;

/**
 * How the client plays HLS with fMP4 segments: through Media Source Extensions (hls.js), with
 * the media element itself, or not at all (only direct play is possible).
 */
export type HlsDelivery = "mse" | "native" | "none";

/** A container the client plays as a progressive file, with the codecs it plays inside it. */
export interface DirectPlayProfile {
  container: MediaIdentifier;
  videoCodecs: MediaIdentifier[];
  audioCodecs: MediaIdentifier[];
}

/** Codecs the client plays inside HLS with fMP4 segments. */
export interface HlsProfile {
  delivery: HlsDelivery;
  videoCodecs: MediaIdentifier[];
  audioCodecs: MediaIdentifier[];
}

/** Extra limits of the client for one codec; a codec without an entry has none. */
export interface CodecLimit {
  codec: MediaIdentifier;
  /** Highest bit depth the client decodes for this codec; omitted when unlimited. */
  maxBitDepth?: number;
  /** Codec profiles the client decodes, as ffprobe names them; omitted when every profile plays. */
  profiles?: string[];
}

/** Everything the server's decision engine needs to know about what a client plays. */
export interface ClientProfile {
  /** Free text naming the client, for logs and the stream-info overlay (at most 64 characters). */
  name: string;
  directPlay: DirectPlayProfile[];
  hls: HlsProfile;
  codecLimits?: CodecLimit[];
}
