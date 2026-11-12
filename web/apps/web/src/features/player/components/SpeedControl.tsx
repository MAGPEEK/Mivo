import { useId } from "react";
import { useTranslation } from "react-i18next";

import { isPlaybackSpeed, playbackSpeeds, type PlaybackSpeed } from "../lib/playbackSpeed";
import { playerNamespace } from "../locales";

/** Props of {@link SpeedControl}. */
export interface SpeedControlProps {
  readonly value: PlaybackSpeed;
  readonly onChange: (speed: PlaybackSpeed) => void;
}

/** The text of a speed step: "1×", "0.75×". */
export function speedLabel(speed: PlaybackSpeed): string {
  return `${String(speed)}×`;
}

/**
 * The playback speed as a labelled list (0.5× to 2×). Reusable: the audiobook player (M5-73)
 * offers the same control. A native select keeps keyboard and screen-reader handling right.
 */
export function SpeedControl({ value, onChange }: SpeedControlProps) {
  const { t } = useTranslation(playerNamespace);
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm text-text">
        {t("options.speed.label")}
      </label>
      <select
        id={id}
        value={String(value)}
        onChange={(event) => {
          const next = Number(event.target.value);
          if (isPlaybackSpeed(next)) {
            onChange(next);
          }
        }}
        className="rounded-md border border-divider bg-surface px-2 py-1 text-sm text-text"
      >
        {playbackSpeeds.map((speed) => (
          <option key={speed} value={String(speed)}>
            {speedLabel(speed)}
          </option>
        ))}
      </select>
    </div>
  );
}
