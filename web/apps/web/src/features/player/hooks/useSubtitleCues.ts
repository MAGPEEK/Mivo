import {
  activeCues,
  subtitleFileTime,
  type PlaybackSession,
  type SubtitleTrack,
} from "@mivo/player-core";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useSyncExternalStore } from "react";

import { subtitleCuesQueryOptions } from "../api/subtitleQueries";

/** Media events after which the cue to show may have changed. */
const cueEvents = ["timeupdate", "seeked", "loadeddata"] as const;

/**
 * The text of the cues to show now (PL-12): the WebVTT of the chosen text track is loaded once,
 * and the cues that cover the playing position are matched on the media element's clock events.
 * Empty without a track, while the track loads, when it fails to load and between cues. The
 * lines of one cue are separated by line breaks.
 */
export function useSubtitleCues(
  video: HTMLVideoElement | null,
  session: PlaybackSession | undefined,
  track: SubtitleTrack | undefined,
): string {
  const url = track?.url;
  const cues = useQuery({
    ...subtitleCuesQueryOptions(session?.fileId ?? "", track?.id ?? "", url ?? ""),
    enabled: session !== undefined && track !== undefined && url !== undefined,
  }).data;
  const subscribe = useCallback(
    (onChange: () => void): (() => void) => {
      for (const event of cueEvents) {
        video?.addEventListener(event, onChange);
      }
      return () => {
        for (const event of cueEvents) {
          video?.removeEventListener(event, onChange);
        }
      };
    },
    [video],
  );
  const currentText = (): string => {
    if (video === null || session === undefined || cues === undefined) {
      return "";
    }
    const fileTime = subtitleFileTime(session, video.currentTime);
    return activeCues(cues, fileTime)
      .map((cue) => cue.text)
      .join("\n");
  };
  return useSyncExternalStore(subscribe, currentText);
}
