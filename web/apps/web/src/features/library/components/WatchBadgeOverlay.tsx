import { useTranslation } from "react-i18next";

import type { WatchBadges } from "../../../shared/lib/watchState";
import { CountBadge, ProgressBar, WatchedBadge } from "../../../shared/ui";
import { libraryNamespace } from "../locales";

/** Props of {@link WatchBadgeOverlay}. */
export interface WatchBadgeOverlayProps {
  readonly badges: WatchBadges;
  /**
   * Leaves the top right corner to another badge, e.g. the "new episodes" count of a row card;
   * only the progress bar shows then.
   */
  readonly cornerTaken?: boolean;
}

/**
 * The watch state of an item over its artwork (LB-42): watched corner, unwatched episode count
 * and progress bar, each with its name in the UI language. Renders nothing without state. Put
 * it into a `relative` box around the artwork, e.g. the overlay of a card.
 */
export function WatchBadgeOverlay({ badges, cornerTaken = false }: WatchBadgeOverlayProps) {
  const { t } = useTranslation(libraryNamespace);
  const { watched, unwatchedCount, progressPercent } = badges;
  return (
    <>
      {!cornerTaken && watched && <WatchedBadge label={t("watch.watched")} />}
      {!cornerTaken && unwatchedCount !== undefined && (
        <CountBadge
          count={unwatchedCount}
          label={t("watch.unwatchedEpisodes", { count: unwatchedCount })}
        />
      )}
      {progressPercent !== undefined && (
        <ProgressBar
          percent={progressPercent}
          label={t("watch.progress", { percent: progressPercent })}
        />
      )}
    </>
  );
}
