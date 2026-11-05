import { useTranslation } from "react-i18next";

import { ExitFullscreenIcon, FullscreenIcon } from "../../../shared/ui";
import { playerNamespace } from "../locales";
import { iconButtonClass } from "./playerStyles";

/** Props of {@link FullscreenButton}. */
export interface FullscreenButtonProps {
  readonly active: boolean;
  readonly onToggle: () => void;
}

/** Enters or leaves full screen; render it only where the browser supports full screen. */
export function FullscreenButton({ active, onToggle }: FullscreenButtonProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-label={active ? t("controls.exitFullscreen") : t("controls.enterFullscreen")}
      className={iconButtonClass}
    >
      {active ? <ExitFullscreenIcon className="size-6" /> : <FullscreenIcon className="size-6" />}
    </button>
  );
}
