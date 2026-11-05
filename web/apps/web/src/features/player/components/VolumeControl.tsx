import { useTranslation } from "react-i18next";

import { MutedIcon, VolumeIcon } from "../../../shared/ui";
import type { MediaState } from "../lib/mediaState";
import { playerNamespace } from "../locales";
import { iconButtonClass } from "./playerStyles";

/** Props of {@link VolumeControl}. */
export interface VolumeControlProps {
  readonly media: Pick<MediaState, "muted" | "volume">;
  readonly onToggleMute: () => void;
  /** Sets the volume (0 to 1); 0 mutes, anything above unmutes. */
  readonly onVolumeChange: (volume: number) => void;
}

const volumeStep = 0.05;
const percent = 100;

/**
 * Mute button and volume slider (PL-04). The slider is a native range input, so arrow keys,
 * Page Up/Down, Home and End work; the device keeps the value (usePersistedVolume).
 */
export function VolumeControl({ media, onToggleMute, onVolumeChange }: VolumeControlProps) {
  const { t } = useTranslation(playerNamespace);
  const { muted, volume } = media;
  const shownVolume = muted ? 0 : volume;
  return (
    <div className="flex items-center gap-1">
      <button
        type="button"
        onClick={onToggleMute}
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
          onVolumeChange(Number(event.currentTarget.value));
        }}
        className="w-20 cursor-pointer accent-accent"
      />
    </div>
  );
}
