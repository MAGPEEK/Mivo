import type { ItemDetail } from "@mivo/api-client";
import {
  audioTracks,
  qualityOptions,
  subtitleTracks,
  type AudioTrack,
  type QualityOption,
  type SubtitleTrack,
} from "@mivo/player-core";

/** What the playback options offer for the file that plays. */
export interface PlayerTracks {
  readonly audio: readonly AudioTrack[];
  readonly subtitles: readonly SubtitleTrack[];
  /** The original and the ladder steps below the source (ADR 0030 §6). */
  readonly quality: readonly QualityOption[];
}

/**
 * The tracks and quality steps of `fileId` in `item`. A file the item does not list offers
 * nothing but the original: the server decides what such a request means.
 */
export function playerTracks(item: ItemDetail, fileId: string): PlayerTracks {
  const file = item.files.find((candidate) => candidate.id === fileId);
  if (file === undefined) {
    return { audio: [], subtitles: [], quality: qualityOptions(undefined, undefined) };
  }
  const videoHeight = file.height ?? file.streams.find((stream) => stream.kind === "video")?.height;
  return {
    audio: audioTracks(file),
    subtitles: subtitleTracks(file),
    quality: qualityOptions(file.bitRate, videoHeight),
  };
}
