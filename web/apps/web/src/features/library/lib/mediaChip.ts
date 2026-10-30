import type { MediaSummary } from "@mivo/api-client";
import { codecLabel, containerLabel } from "@mivo/player-core";

const chipSeparator = " · ";

/**
 * The technical chip of a card, e.g. `MKV · H.264 · AAC`: container, video and audio codec,
 * each only when known. Empty when nothing is known (an unprobed file).
 */
export function mediaChipLabel(media: MediaSummary | undefined): string {
  if (media === undefined) {
    return "";
  }
  const labels = [
    media.container === undefined ? undefined : containerLabel(media.container),
    media.videoCodec === undefined ? undefined : codecLabel(media.videoCodec),
    media.audioCodec === undefined ? undefined : codecLabel(media.audioCodec),
  ];
  return labels.filter((label) => label !== undefined).join(chipSeparator);
}
