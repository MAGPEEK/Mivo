import { useTranslation } from "react-i18next";

import { secondaryButtonClass, ShuffleIcon } from "../../../shared/ui";
import { useAlbumShuffle } from "../hooks/useAlbumTracks";
import { musicNamespace } from "../locales";

/** Props of {@link AlbumShuffleButton}. */
export interface AlbumShuffleButtonProps {
  readonly albumId: string;
}

/**
 * Plays the album in random order. It uses `aria-disabled` while the tracks load or none can
 * be played, so it keeps its keyboard focus.
 */
export function AlbumShuffleButton({ albumId }: AlbumShuffleButtonProps) {
  const { t } = useTranslation(musicNamespace);
  const { enabled, shuffle } = useAlbumShuffle(albumId);
  return (
    <button
      type="button"
      aria-disabled={!enabled}
      onClick={enabled ? shuffle : undefined}
      className={`inline-flex items-center gap-2 ${secondaryButtonClass}`}
    >
      <ShuffleIcon className="size-5" />
      {t("album.shuffle")}
    </button>
  );
}
