/** Container formats the server can deliver and a client may be able to play natively. */
export type ContainerFormat = "mp4" | "webm" | "matroska" | "mpegts" | "hls";

/**
 * How confident a client is that it can play a media type. Mirrors the three answers of
 * `HTMLMediaElement.canPlayType`, with the empty answer named explicitly.
 */
export type SupportLevel = "unsupported" | "maybe" | "probably";

/** A container the client is asked about, together with the MIME type used for the probe. */
export interface ContainerCandidate {
  readonly container: ContainerFormat;
  readonly mimeType: string;
}

/** The probe result for one container candidate. */
export interface ContainerSupport extends ContainerCandidate {
  readonly level: SupportLevel;
}

/**
 * What a client reports about its playback abilities. The server's playback decision uses it
 * to choose between direct play, direct stream and transcode.
 */
export interface ClientCapabilities {
  readonly containers: readonly ContainerSupport[];
}
