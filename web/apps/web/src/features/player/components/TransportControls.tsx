import type { ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  PauseIcon,
  PlayIcon,
  SkipBackwardIcon,
  SkipForwardIcon,
  SkipNextIcon,
  SkipPreviousIcon,
  StopIcon,
} from "../../../shared/ui";
import type { PlayerActions, PlayerView } from "../hooks/usePlayer";
import { playerNamespace } from "../locales";
import { iconButtonClass, largeIconButtonClass } from "./playerStyles";

/** Props of {@link TransportControls}. */
export interface TransportControlsProps {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
  readonly playButtonRef: RefObject<HTMLButtonElement | null>;
}

// Unavailable buttons are aria-disabled instead of disabled: a focused Next that becomes
// unavailable (the queue reached its end) keeps focus instead of dropping it to the document.
const unavailableClass =
  "aria-disabled:cursor-not-allowed aria-disabled:opacity-40 aria-disabled:hover:bg-transparent";

interface TransportButtonProps {
  readonly label: string;
  readonly onClick: () => void;
  readonly unavailable?: boolean;
  readonly children: ReactNode;
}

function TransportButton({ label, onClick, unavailable = false, children }: TransportButtonProps) {
  return (
    <button
      type="button"
      onClick={() => {
        if (!unavailable) {
          onClick();
        }
      }}
      aria-disabled={unavailable}
      aria-label={label}
      className={`${iconButtonClass} ${unavailableClass}`}
    >
      {children}
    </button>
  );
}

/**
 * The centre of the control bar (PL-03): previous, 10 s back, play/pause, 30 s forward, next
 * and stop. Previous and next stay unavailable while there is no queue to move through.
 */
export function TransportControls({ view, actions, playButtonRef }: TransportControlsProps) {
  const { t } = useTranslation(playerNamespace);
  const { paused } = view.media;
  return (
    <div className="flex items-center justify-center gap-1">
      <TransportButton
        label={t("controls.previous")}
        onClick={actions.previous}
        unavailable={!view.queue.canGoPrevious}
      >
        <SkipPreviousIcon className="size-5" />
      </TransportButton>
      <TransportButton label={t("controls.skipBackward")} onClick={actions.skipBackward}>
        <SkipBackwardIcon className="size-6" />
      </TransportButton>
      <button
        ref={playButtonRef}
        type="button"
        onClick={actions.togglePlay}
        aria-label={paused ? t("controls.play") : t("controls.pause")}
        className={largeIconButtonClass}
      >
        {paused ? <PlayIcon className="size-7" /> : <PauseIcon className="size-7" />}
      </button>
      <TransportButton label={t("controls.skipForward")} onClick={actions.skipForward}>
        <SkipForwardIcon className="size-6" />
      </TransportButton>
      <TransportButton
        label={t("controls.next")}
        onClick={actions.next}
        unavailable={!view.queue.canGoNext}
      >
        <SkipNextIcon className="size-5" />
      </TransportButton>
      <TransportButton label={t("controls.stop")} onClick={actions.stop}>
        <StopIcon className="size-5" />
      </TransportButton>
    </div>
  );
}
