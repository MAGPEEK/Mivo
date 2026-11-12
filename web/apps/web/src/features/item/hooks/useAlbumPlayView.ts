import { useTranslation } from "react-i18next";

import { useAlbumPlayTrack } from "../../music";
import type { PlayView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/**
 * What the album's Play button does: open the player for its first playable track. The
 * album itself has no file, so its `playItemId` cannot be played (BACKLOG, M3-13h).
 */
export function useAlbumPlayView(albumId: string): PlayView {
  const { t } = useTranslation(itemNamespace);
  const track = useAlbumPlayTrack(albumId);
  if (track.status === "pending") {
    return { kind: "unavailable", explanation: t("play.unavailable.tracksLoading") };
  }
  if (track.status === "error") {
    return { kind: "unavailable", explanation: t("play.unavailable.tracksFailed") };
  }
  return track.trackId === undefined
    ? { kind: "unavailable", explanation: t("play.unavailable.noTracks") }
    : { kind: "playable", itemId: track.trackId, queue: track.queueIds };
}
