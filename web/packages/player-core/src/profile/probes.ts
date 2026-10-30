// Browser APIs reach player-core only through these interfaces, so the package reads no DOM
// globals and the TV app can inject its own implementations. A media element, the MediaSource
// constructor and `navigator.mediaCapabilities` satisfy them directly.

/** Answers like `HTMLMediaElement.canPlayType`: `"probably"`, `"maybe"` or `""`. */
export interface MediaElementProbe {
  canPlayType(mimeType: string): string;
}

/** Answers like the static `MediaSource.isTypeSupported`. */
export interface MediaSourceProbe {
  isTypeSupported(mimeType: string): boolean;
}

/** `"file"` for progressive and native playback, `"media-source"` for playback through MSE. */
export type DecodingType = "file" | "media-source";

/** Video part of a decoding query, a subset of the DOM's `VideoConfiguration`. */
export interface VideoDecodingConfiguration {
  readonly contentType: string;
  readonly width: number;
  readonly height: number;
  /** Bits per second. */
  readonly bitrate: number;
  readonly framerate: number;
}

/** Audio part of a decoding query, a subset of the DOM's `AudioConfiguration`. */
export interface AudioDecodingConfiguration {
  readonly contentType: string;
}

/** A decoding query, a subset of the DOM's `MediaDecodingConfiguration`. */
export interface DecodingConfiguration {
  readonly type: DecodingType;
  readonly video?: VideoDecodingConfiguration;
  readonly audio?: AudioDecodingConfiguration;
}

/** The part of the DOM's `MediaCapabilitiesDecodingInfo` that the profile needs. */
export interface DecodingResult {
  readonly supported: boolean;
}

/** Answers like `navigator.mediaCapabilities.decodingInfo`; may reject for unknown types. */
export interface MediaCapabilitiesProbe {
  decodingInfo(configuration: DecodingConfiguration): Promise<DecodingResult>;
}

/**
 * Starts a one-shot timer like `setTimeout` and returns a function that cancels it. Injected
 * because player-core has no timer globals; it bounds `decodingInfo` calls that never settle.
 */
export type StartTimer = (milliseconds: number, onTimeout: () => void) => () => void;

/** The browser APIs a client profile is detected from. */
export interface ClientProbes {
  readonly mediaElement: MediaElementProbe;
  readonly startTimer: StartTimer;
  /** Absent when the browser has no Media Source Extensions. */
  readonly mediaSource?: MediaSourceProbe | undefined;
  /** Absent when the browser has no Media Capabilities API. */
  readonly mediaCapabilities?: MediaCapabilitiesProbe | undefined;
}
