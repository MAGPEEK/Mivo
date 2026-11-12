import { useTranslation } from "react-i18next";

import type { PlayerView } from "../hooks/usePlayer";
import { playerNamespace } from "../locales";
import { SubtitleLayer } from "./SubtitleLayer";

/** Props of {@link PlayerVideo}. */
export interface PlayerVideoProps {
  readonly view: Pick<PlayerView, "subtitleText" | "options" | "controlsVisible">;
  /** The `ref` of the `<video>` element. */
  readonly videoRef: (element: HTMLVideoElement | null) => void;
}

/**
 * The video and the subtitle the player draws over it. Subtitles are drawn by
 * {@link SubtitleLayer} from the server's WebVTT, not as `<track>` elements: the chosen track
 * changes during playback (PL-12).
 */
export function PlayerVideo({ view, videoRef }: PlayerVideoProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y/media-has-caption -- see the comment above */}
      <video
        ref={videoRef}
        aria-label={t("video")}
        className="absolute inset-0 size-full bg-background object-contain"
        playsInline
      />
      <SubtitleLayer
        text={view.subtitleText}
        size={view.options.subtitleSize}
        controlsVisible={view.controlsVisible}
      />
    </>
  );
}
