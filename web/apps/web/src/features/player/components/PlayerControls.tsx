import type { ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";

import { InfoIcon, SettingsIcon } from "../../../shared/ui";
import { playerControlsMarker } from "../hooks/useControlsVisibility";
import type { PlayerActions, PlayerView } from "../hooks/usePlayer";
import { playerNamespace } from "../locales";
import { controlBarPaddingClass, fadeClass, iconButtonClass } from "./playerStyles";
import { QueueModeControls } from "./QueueModeControls";
import { Scrubber } from "./Scrubber";
import { TitleBlock } from "./TitleBlock";
import { TransportControls } from "./TransportControls";
import { VolumeControl } from "./VolumeControl";

/**
 * Places in the control bar that later tasks fill (PL-04): the mode indicator (M5-62), the
 * More menu (M5-63) and the queue panel button (M5-65). The playback options button
 * (PL-10 … PL-13) is part of the bar itself.
 */
export interface PlayerSlots {
  readonly modeIndicator?: ReactNode;
  readonly more?: ReactNode;
  readonly queue?: ReactNode;
}

/** Props of {@link PlayerControls}. */
export interface PlayerControlsProps {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
  readonly playButtonRef: RefObject<HTMLButtonElement | null>;
  readonly streamInfoToggleRef: RefObject<HTMLButtonElement | null>;
  readonly optionsToggleRef: RefObject<HTMLButtonElement | null>;
  readonly slots?: PlayerSlots | undefined;
}

/**
 * The bar below the video: the scrubber, then the title block on the left, the transport in
 * the centre and the queue, sound and panel controls on the right. On narrow screens the three
 * groups stack, each centred (the phone layout of the prototype).
 */
export function PlayerControls({
  view,
  actions,
  playButtonRef,
  streamInfoToggleRef,
  optionsToggleRef,
  slots = {},
}: PlayerControlsProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div
      {...playerControlsMarker}
      role="group"
      aria-label={t("controls.label")}
      className={`absolute inset-x-0 bottom-0 bg-linear-to-t from-background/90 to-transparent ${controlBarPaddingClass} ${fadeClass(view.controlsVisible)}`}
    >
      <Scrubber
        position={view.position}
        duration={view.duration}
        producedRanges={view.media.producedRanges}
        onSeek={actions.seek}
      />
      <div className="mt-2 grid grid-cols-1 items-center gap-2 text-center md:grid-cols-[1fr_auto_1fr] md:text-start">
        <TitleBlock item={view.item} position={view.position} duration={view.duration} />
        <TransportControls view={view} actions={actions} playButtonRef={playButtonRef} />
        <SideControls
          view={view}
          actions={actions}
          streamInfoToggleRef={streamInfoToggleRef}
          optionsToggleRef={optionsToggleRef}
          slots={slots}
        />
      </div>
    </div>
  );
}

type SideControlsProps = Omit<PlayerControlsProps, "playButtonRef" | "slots"> & {
  readonly slots: PlayerSlots;
};

/** The right group (PL-04): mode indicator, stream info, More, repeat, shuffle, panels, sound. */
function SideControls({
  view,
  actions,
  streamInfoToggleRef,
  optionsToggleRef,
  slots,
}: SideControlsProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div className="flex flex-wrap items-center justify-center gap-1 md:justify-end">
      {slots.modeIndicator}
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
      {slots.more}
      <QueueModeControls
        queue={view.queue}
        onCycleRepeat={actions.cycleRepeat}
        onToggleShuffle={actions.toggleShuffle}
      />
      <button
        ref={optionsToggleRef}
        type="button"
        onClick={actions.options.toggle}
        aria-haspopup="dialog"
        aria-expanded={view.options.open}
        aria-label={t("controls.options")}
        className={iconButtonClass}
      >
        <SettingsIcon className="size-6" />
      </button>
      {slots.queue}
      <VolumeControl
        media={view.media}
        onToggleMute={actions.toggleMute}
        onVolumeChange={actions.setVolume}
      />
    </div>
  );
}
