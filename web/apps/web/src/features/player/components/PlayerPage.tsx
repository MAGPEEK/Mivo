import { useTranslation } from "react-i18next";

import { usePlayer } from "../hooks/usePlayer";
import type { PlayerSearch } from "../lib/playerSearch";
import { playerNamespace } from "../locales";
import { PlayerControls } from "./PlayerControls";
import { PlayerNotice } from "./PlayerNotice";
import { PlayerTopBar } from "./PlayerTopBar";
import { StreamInfoOverlay } from "./StreamInfoOverlay";

/** Props of {@link PlayerPage}, from the route `/items/$itemId/play` and its search. */
export interface PlayerPageProps {
  readonly itemId: string;
  /** The validated search: file to play and start position (`t`); both untrusted. */
  readonly search: PlayerSearch;
}

/**
 * The full-screen player (task M2-30): the video with its own control bar, a scrubber over the
 * whole file and the stream-info overlay. Native `controls` stay off, because the `EVENT`
 * playlist of an HLS stream would show only its produced part (ADR 0024).
 */
export function PlayerPage({ itemId, search }: PlayerPageProps) {
  const { t } = useTranslation(playerNamespace);
  // Destructured: the React compiler treats an object holding refs as a ref.
  const {
    view,
    actions,
    videoRef,
    containerRef,
    playButtonRef,
    streamInfoToggleRef,
    streamInfoRegionRef,
    streamInfoHeadingRef,
  } = usePlayer({ itemId, fileId: search.fileId, startAt: search.t });
  return (
    <div
      ref={containerRef}
      className={`relative h-dvh w-full overflow-hidden bg-background ${view.controlsVisible ? "" : "cursor-none"}`}
    >
      {/* The server offers no subtitle tracks before milestone 4, so there is nothing to add. */}
      {/* eslint-disable-next-line jsx-a11y/media-has-caption -- no caption tracks until M4 */}
      <video
        ref={videoRef}
        aria-label={t("video")}
        className="absolute inset-0 size-full bg-background object-contain"
        playsInline
      />
      <PlayerNotice notice={view.notice} onRetry={actions.retry} onOpenItem={actions.openItem} />
      <PlayerTopBar title={view.title} visible={view.controlsVisible} onExit={actions.exit} />
      <PlayerControls
        view={view}
        actions={actions}
        playButtonRef={playButtonRef}
        streamInfoToggleRef={streamInfoToggleRef}
      />
      {view.streamInfoOpen ? (
        <StreamInfoOverlay
          info={view.streamInfo}
          regionRef={streamInfoRegionRef}
          headingRef={streamInfoHeadingRef}
          onClose={actions.closeStreamInfo}
        />
      ) : null}
    </div>
  );
}
