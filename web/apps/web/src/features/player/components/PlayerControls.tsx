import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  ExitFullscreenIcon,
  FullscreenIcon,
  InfoIcon,
  MutedIcon,
  PauseIcon,
  PlayIcon,
  VolumeIcon,
} from "../../../shared/ui";
import { playerControlsMarker } from "../hooks/useControlsVisibility";
import type { PlayerActions, PlayerView } from "../hooks/usePlayer";
import { displayedClock } from "../lib/playbackTime";
import { playerNamespace } from "../locales";
import { fadeClass, iconButtonClass } from "./playerStyles";
import { Scrubber } from "./Scrubber";

/** Props of {@link PlayerControls}. */
export interface PlayerControlsProps {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
  readonly playButtonRef: RefObject<HTMLButtonElement | null>;
  readonly streamInfoToggleRef: RefObject<HTMLButtonElement | null>;
}

const volumeStep = 0.05;
const percent = 100;

/** The bar below the video: scrubber, play/pause, sound, time, stream info and full screen. */
export function PlayerControls({
  view,
  actions,
  playButtonRef,
  streamInfoToggleRef,
}: PlayerControlsProps) {
  const { t } = useTranslation(playerNamespace);
  const { media } = view;
  return (
    <div
      {...playerControlsMarker}
      role="group"
      aria-label={t("controls.label")}
      className={`absolute inset-x-0 bottom-0 bg-linear-to-t from-background/90 to-transparent px-4 pt-12 pb-4 ${fadeClass(view.controlsVisible)}`}
    >
      <Scrubber
        position={view.position}
        duration={view.duration}
        producedRanges={media.producedRanges}
        onSeek={actions.seek}
      />
      <div className="mt-2 flex items-center gap-2">
        <button
          ref={playButtonRef}
          type="button"
          onClick={actions.togglePlay}
          aria-label={media.paused ? t("controls.play") : t("controls.pause")}
          className={iconButtonClass}
        >
          {media.paused ? <PlayIcon className="size-6" /> : <PauseIcon className="size-6" />}
        </button>
        <VolumeControls view={view} actions={actions} />
        <TimeText position={view.position} duration={view.duration} />
        <div className="ms-auto flex items-center gap-2">
          <button
            ref={streamInfoToggleRef}
            type="button"
            onClick={actions.toggleStreamInfo}
            aria-expanded={view.streamInfoOpen}
            aria-label={t("controls.streamInfo")}
            className={iconButtonClass}
          >
            <InfoIcon className="size-6" />
          </button>
          {view.fullscreenSupported ? <FullscreenButton view={view} actions={actions} /> : null}
        </div>
      </div>
    </div>
  );
}

interface ControlProps {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
}

function VolumeControls({ view, actions }: ControlProps) {
  const { t } = useTranslation(playerNamespace);
  const { muted, volume } = view.media;
  const shownVolume = muted ? 0 : volume;
  return (
    <div className="flex items-center gap-1">
      <button
        type="button"
        onClick={actions.toggleMute}
        aria-label={muted ? t("controls.unmute") : t("controls.mute")}
        className={iconButtonClass}
      >
        {muted ? <MutedIcon className="size-6" /> : <VolumeIcon className="size-6" />}
      </button>
      <input
        type="range"
        min={0}
        max={1}
        step={volumeStep}
        value={shownVolume}
        aria-label={t("controls.volume")}
        aria-valuetext={t("controls.volumeValue", { percent: Math.round(shownVolume * percent) })}
        onChange={(event) => {
          actions.setVolume(Number(event.currentTarget.value));
        }}
        className="w-24 cursor-pointer accent-accent"
      />
    </div>
  );
}

function TimeText({ position, duration }: Pick<PlayerView, "position" | "duration">) {
  const { t } = useTranslation(playerNamespace);
  const clock = displayedClock(position, duration);
  return (
    <p className="px-2 text-sm whitespace-nowrap tabular-nums">
      {clock.duration === undefined
        ? clock.position
        : t("controls.time", { position: clock.position, duration: clock.duration })}
    </p>
  );
}

function FullscreenButton({ view, actions }: ControlProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <button
      type="button"
      onClick={actions.toggleFullscreen}
      aria-label={
        view.fullscreenActive ? t("controls.exitFullscreen") : t("controls.enterFullscreen")
      }
      className={iconButtonClass}
    >
      {view.fullscreenActive ? (
        <ExitFullscreenIcon className="size-6" />
      ) : (
        <FullscreenIcon className="size-6" />
      )}
    </button>
  );
}
