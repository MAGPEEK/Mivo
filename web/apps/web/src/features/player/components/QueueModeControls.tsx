import { useTranslation } from "react-i18next";

import { RepeatIcon, RepeatOneIcon, ShuffleIcon } from "../../../shared/ui";
import type { PlayerQueueView } from "../hooks/usePlayerQueue";
import { playerNamespace } from "../locales";
import { toggleButtonClass } from "./playerStyles";

/** Props of {@link QueueModeControls}. */
export interface QueueModeControlsProps {
  readonly queue: PlayerQueueView;
  readonly onCycleRepeat: () => void;
  readonly onToggleShuffle: () => void;
}

/**
 * Repeat (off, all, one) and shuffle (PL-04). Both are toggle buttons with `aria-pressed`; the
 * repeat mode also shows as a tooltip and in the icon. The setting changed last is announced
 * in a polite live region. It exists from the start, so screen readers pick up changes, and
 * has no `role="status"`: that role belongs to the playback notice.
 */
export function QueueModeControls({
  queue,
  onCycleRepeat,
  onToggleShuffle,
}: QueueModeControlsProps) {
  const { t } = useTranslation(playerNamespace);
  const repeatState = t(`controls.repeatState.${queue.repeat}`);
  const shuffleState = queue.shuffle ? t("controls.shuffleOn") : t("controls.shuffleOff");
  const announcements = { repeat: repeatState, shuffle: shuffleState };
  return (
    <>
      <button
        type="button"
        onClick={onCycleRepeat}
        aria-pressed={queue.repeat !== "off"}
        aria-label={t("controls.repeat")}
        title={repeatState}
        className={toggleButtonClass(queue.repeat !== "off")}
      >
        {queue.repeat === "one" ? (
          <RepeatOneIcon className="size-5" />
        ) : (
          <RepeatIcon className="size-5" />
        )}
      </button>
      <button
        type="button"
        onClick={onToggleShuffle}
        aria-pressed={queue.shuffle}
        aria-label={t("controls.shuffle")}
        title={shuffleState}
        className={toggleButtonClass(queue.shuffle)}
      >
        <ShuffleIcon className="size-5" />
      </button>
      <p aria-live="polite" className="sr-only">
        {queue.announcement === undefined ? "" : announcements[queue.announcement]}
      </p>
    </>
  );
}
