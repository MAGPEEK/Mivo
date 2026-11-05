import { useTranslation } from "react-i18next";

import { BackIcon, MinimiseIcon } from "../../../shared/ui";
import { playerControlsMarker } from "../hooks/useControlsVisibility";
import type { PlayerActions, PlayerView } from "../hooks/usePlayer";
import { playerNamespace } from "../locales";
import { FullscreenButton } from "./FullscreenButton";
import { fadeClass, iconButtonClass, topBarPaddingClass } from "./playerStyles";

/** Props of {@link PlayerTopBar}. */
export interface PlayerTopBarProps {
  readonly view: Pick<PlayerView, "controlsVisible" | "fullscreenSupported" | "fullscreenActive">;
  readonly actions: Pick<PlayerActions, "exit" | "minimise" | "toggleFullscreen">;
}

/** The bar above the video (PL-01): back out of the player, Minimise and full screen. */
export function PlayerTopBar({ view, actions }: PlayerTopBarProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div
      {...playerControlsMarker}
      className={`absolute inset-x-0 top-0 flex items-center gap-1 bg-linear-to-b from-background/90 to-transparent ${topBarPaddingClass} ${fadeClass(view.controlsVisible)}`}
    >
      <button
        type="button"
        onClick={actions.exit}
        aria-label={t("back")}
        className={iconButtonClass}
      >
        <BackIcon className="size-6" />
      </button>
      <button
        type="button"
        onClick={actions.minimise}
        aria-label={t("controls.minimise")}
        className={iconButtonClass}
      >
        <MinimiseIcon className="size-6" />
      </button>
      <div className="ms-auto">
        {view.fullscreenSupported ? (
          <FullscreenButton active={view.fullscreenActive} onToggle={actions.toggleFullscreen} />
        ) : null}
      </div>
    </div>
  );
}
