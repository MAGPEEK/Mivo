import { usePlayer } from "../hooks/usePlayer";
import type { PlayerSearch } from "../lib/playerSearch";
import { PlayerControls, type PlayerSlots } from "./PlayerControls";
import { PlaybackOptionsPanel } from "./PlaybackOptionsPanel";
import { PlayerNotice } from "./PlayerNotice";
import { PlayerToasts } from "./PlayerToasts";
import { PlayerTopBar } from "./PlayerTopBar";
import { StreamInfoOverlay } from "./StreamInfoOverlay";
import { PlayerVideo } from "./PlayerVideo";

/** Props of {@link PlayerPage}, from the route `/items/$itemId/play` and its search. */
export interface PlayerPageProps {
  readonly itemId: string;
  /** The validated search: file to play and start position (`t`); both untrusted. */
  readonly search: PlayerSearch;
  /** Controls later tasks add to the control bar (mode indicator, More, queue). */
  readonly slots?: PlayerSlots;
}

/**
 * The full-screen player (task M2-30): the video with its own control bar, a scrubber over the
 * whole file and the stream-info overlay. Native `controls` stay off, because the `EVENT`
 * playlist of an HLS stream would show only its produced part (ADR 0024).
 */
export function PlayerPage({ itemId, search, slots }: PlayerPageProps) {
  // Destructured: the React compiler treats an object holding refs as a ref.
  const {
    view,
    actions,
    videoRef,
    containerRef,
    playButtonRef,
    streamInfoToggleRef,
    optionsToggleRef,
    streamInfoRegionRef,
    streamInfoHeadingRef,
  } = usePlayer({ itemId, fileId: search.fileId, startAt: search.t });
  return (
    <div
      ref={containerRef}
      className={`relative h-dvh w-full overflow-hidden bg-background ${view.controlsVisible ? "" : "cursor-none"}`}
    >
      <PlayerVideo view={view} videoRef={videoRef} />
      <PlayerNotice notice={view.notice} onRetry={actions.retry} onOpenItem={actions.openItem} />
      <PlayerTopBar view={view} actions={actions} />
      <PlayerControls
        view={view}
        actions={actions}
        playButtonRef={playButtonRef}
        streamInfoToggleRef={streamInfoToggleRef}
        optionsToggleRef={optionsToggleRef}
        slots={slots}
      />
      <PlayerToasts toasts={view.toasts} onDismiss={actions.dismissToasts} />
      {view.options.open ? <PlaybackOptionsPanel view={view} actions={actions} /> : null}
      {view.streamInfoOpen ? (
        <StreamInfoOverlay
          info={view.streamInfo}
          networkStats={view.networkStats}
          regionRef={streamInfoRegionRef}
          headingRef={streamInfoHeadingRef}
          onClose={actions.closeStreamInfo}
        />
      ) : null}
    </div>
  );
}
