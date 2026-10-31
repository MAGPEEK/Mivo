import { useTranslation } from "react-i18next";

import { BackIcon } from "../../../shared/ui";
import { playerControlsMarker } from "../hooks/useControlsVisibility";
import { playerNamespace } from "../locales";
import { fadeClass, iconButtonClass } from "./playerStyles";

/** Props of {@link PlayerTopBar}. */
export interface PlayerTopBarProps {
  /** Title of the item; the generic heading stands in until it has loaded. */
  readonly title: string | undefined;
  readonly visible: boolean;
  readonly onExit: () => void;
}

/** The bar above the video: back out of the player and the item's title as the page heading. */
export function PlayerTopBar({ title, visible, onExit }: PlayerTopBarProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div
      {...playerControlsMarker}
      className={`absolute inset-x-0 top-0 flex items-center gap-3 bg-linear-to-b from-background/90 to-transparent px-4 pt-4 pb-12 ${fadeClass(visible)}`}
    >
      <button type="button" onClick={onExit} aria-label={t("back")} className={iconButtonClass}>
        <BackIcon className="size-6" />
      </button>
      <h1 className="truncate text-xl font-semibold">{title ?? t("heading")}</h1>
    </div>
  );
}
