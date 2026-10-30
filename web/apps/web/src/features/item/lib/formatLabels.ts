import type { MediaStream } from "@mivo/api-client";
import { bitDepthLabel, channelLabel, codecLabel, hdrLabel } from "@mivo/player-core";

// player-core labels audio and video codecs only. Subtitle formats get their common names
// here until player-core covers them; any other codec falls back to codecLabel.
// Map instead of an object literal, so an id such as "constructor" never hits Object.prototype.
const subtitleFormatLabels: ReadonlyMap<string, string> = new Map([
  ["subrip", "SRT"],
  ["webvtt", "WebVTT"],
  ["mov_text", "TX3G"],
  ["hdmv_pgs_subtitle", "PGS"],
  ["dvd_subtitle", "VobSub"],
  ["dvb_subtitle", "DVB"],
]);

// The API's HdrFormat values have player-core labels. Later API versions may send more
// (the enum is open); the known future ones get their usual names, anything else a plain
// "HDR" rather than an upper-cased identifier such as "DOLBY_VISION".
const playerCoreHdrFormats: ReadonlySet<string> = new Set(["hdr10", "hlg"]);
const futureHdrFormatLabels: ReadonlyMap<string, string> = new Map([
  ["dolby_vision", "Dolby Vision"],
  ["hdr10plus", "HDR10+"],
]);
const genericHdrLabel = "HDR";

/** Video bit depths above this are worth showing; 8-bit is the unremarkable default. */
const standardBitDepth = 8;

const labelPartSeparator = " ";

/** Label of a subtitle format, e.g. `subrip` → `SRT`, `hdmv_pgs_subtitle` → `PGS`. */
export function subtitleFormatLabel(codec: string): string {
  return subtitleFormatLabels.get(codec) ?? codecLabel(codec);
}

/** Label of an HDR format, e.g. `hdr10` → `HDR10`, `dolby_vision` → `Dolby Vision`, else `HDR`. */
export function hdrFormatLabel(hdr: string): string {
  if (playerCoreHdrFormats.has(hdr)) {
    return hdrLabel(hdr);
  }
  return futureHdrFormatLabels.get(hdr) ?? genericHdrLabel;
}

/**
 * Label of a video stream, e.g. `HEVC 10-bit HDR10` or `H.264`. Bit depth appears above 8 bit
 * and HDR only when present, so SDR 8-bit video reads as the codec alone. Empty when the stream
 * reports none of these.
 */
export function videoStreamLabel(stream: MediaStream): string {
  const { codec, bitDepth, hdr } = stream;
  return joinParts([
    codec === undefined ? undefined : codecLabel(codec),
    bitDepth !== undefined && bitDepth > standardBitDepth ? bitDepthLabel(bitDepth) : undefined,
    hdr === undefined ? undefined : hdrFormatLabel(hdr),
  ]);
}

/** Label of an audio stream, e.g. `AC-3 5.1`; empty when neither codec nor channels are known. */
export function audioStreamLabel(stream: MediaStream): string {
  const { codec, channels } = stream;
  return joinParts([
    codec === undefined ? undefined : codecLabel(codec),
    channels === undefined ? undefined : channelLabel(channels),
  ]);
}

/** Label of a subtitle stream's format; empty when the codec is unknown. */
export function subtitleStreamLabel(stream: MediaStream): string {
  return stream.codec === undefined ? "" : subtitleFormatLabel(stream.codec);
}

function joinParts(parts: readonly (string | undefined)[]): string {
  return parts.filter((part) => part !== undefined).join(labelPartSeparator);
}
