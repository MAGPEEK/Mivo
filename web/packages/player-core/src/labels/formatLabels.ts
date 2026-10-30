// Display labels for container and codec identifiers (ADR 0023). The library grid, the detail
// page and the stream-info overlay all use these, so a codec never reads differently in two
// places. Labels are technical names and need no translation.

// Maps instead of object literals, so an id such as "constructor" never hits Object.prototype.
const containerLabels: ReadonlyMap<string, string> = new Map([
  ["webm", "WebM"],
  ["ogg", "Ogg"],
  ["mpegts", "MPEG-TS"],
  ["mpeg", "MPEG-PS"],
  ["rm", "RealMedia"],
]);

const codecLabels: ReadonlyMap<string, string> = new Map([
  ["h263", "H.263"],
  ["h264", "H.264"],
  ["hevc", "HEVC"],
  ["mpeg1video", "MPEG-1"],
  ["mpeg2video", "MPEG-2"],
  ["mpeg4", "MPEG-4"],
  ["vc1", "VC-1"],
  ["theora", "Theora"],
  ["prores", "ProRes"],
  ["opus", "Opus"],
  ["vorbis", "Vorbis"],
  ["ac3", "AC-3"],
  ["eac3", "E-AC-3"],
  ["truehd", "TrueHD"],
  ["wmav2", "WMA"],
]);

// ffprobe names every PCM sample format separately (pcm_s16le, pcm_s24be, pcm_f32le, ...).
const pcmCodecPrefix = "pcm_";
const pcmLabel = "PCM";

/** Label of a container short name, e.g. `mkv` → `MKV`, `webm` → `WebM`. */
export function containerLabel(container: string): string {
  return containerLabels.get(container) ?? container.toUpperCase();
}

/** Label of an ffprobe codec name, e.g. `h264` → `H.264`, `pcm_s16le` → `PCM`. */
export function codecLabel(codec: string): string {
  if (codec.startsWith(pcmCodecPrefix)) {
    return pcmLabel;
  }
  return codecLabels.get(codec) ?? codec.toUpperCase();
}
