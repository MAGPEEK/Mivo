import type { MediaFile } from "@mivo/api-client";
import { containerLabel } from "@mivo/player-core";

import { audioStreamLabel, subtitleStreamLabel, videoStreamLabel } from "./formatLabels";
import { primaryAudioStream, primaryVideoStream, streamsOfKind } from "./primaryStreams";

/** Joins the technical chips into the plan's one-line form `MKV · HEVC 10-bit HDR10 · AC-3 5.1`. */
export const chipSeparator = " · ";

// Several subtitle formats share one chip ("SRT/ASS"), so the chip count stays fixed at four.
const subtitleFormatSeparator = "/";

/**
 * Technical chips of a media file in the plan's order: container, primary video, primary audio
 * and the subtitle formats, e.g. `["MKV", "HEVC 10-bit HDR10", "AC-3 5.1", "SRT"]`. A chip is
 * left out when the file does not report it, so a file whose probing failed has none.
 */
export function technicalChips(file: MediaFile): string[] {
  const video = primaryVideoStream(file);
  const audio = primaryAudioStream(file);
  const chips = [
    file.container === undefined ? "" : containerLabel(file.container),
    video === undefined ? "" : videoStreamLabel(video),
    audio === undefined ? "" : audioStreamLabel(audio),
    subtitleFormats(file),
  ];
  return chips.filter((chip) => chip !== "");
}

function subtitleFormats(file: MediaFile): string {
  const labels = streamsOfKind(file, "subtitle")
    .map(subtitleStreamLabel)
    .filter((label) => label !== "");
  return [...new Set(labels)].join(subtitleFormatSeparator);
}
