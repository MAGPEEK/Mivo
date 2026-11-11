// Builders for track tests: only the fields a case is about are spelled out.
import type { AudioTrack, SubtitleTrack } from "../trackModel";

export function audio(index: number, fields: Partial<AudioTrack> = {}): AudioTrack {
  return { index, default: false, commentary: false, ...fields };
}

/** A text track with a WebVTT rendition unless the fields say otherwise. */
export function subtitle(id: string, fields: Partial<SubtitleTrack> = {}): SubtitleTrack {
  return {
    ...imageSubtitle(id),
    format: "subrip",
    textBased: true,
    url: `/subtitles/${id}.vtt`,
    sidecar: true,
    ...fields,
  };
}

/** An image track, which only burn-in can show. */
export function imageSubtitle(id: string, fields: Partial<SubtitleTrack> = {}): SubtitleTrack {
  return {
    id,
    format: "hdmv_pgs_subtitle",
    forced: false,
    sdh: false,
    default: false,
    external: false,
    textBased: false,
    sidecar: false,
    ...fields,
  };
}
