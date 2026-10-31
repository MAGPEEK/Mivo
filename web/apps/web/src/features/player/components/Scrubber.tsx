import type { TimeRange } from "@mivo/player-core";
import { useTranslation } from "react-i18next";

import { useScrubber } from "../hooks/useScrubber";
import { spokenClock } from "../lib/playbackTime";
import { trackPercent, trackSpan } from "../lib/scrubberTrack";
import { playerNamespace } from "../locales";

/** Props of {@link Scrubber}. */
export interface ScrubberProps {
  /** Displayed file position in seconds. */
  readonly position: number;
  /** Probed duration of the whole file; the slider is disabled while it is unknown. */
  readonly duration: number | undefined;
  /** Produced part of an HLS stream, shaded on the track. */
  readonly producedRanges: readonly TimeRange[];
  readonly onSeek: (position: number) => void;
}

/**
 * The position slider over the whole file (ADR 0024). A transparent native range input lies
 * over the drawn track, so pointer, keyboard and screen readers get a real slider; the wrapper
 * draws its focus ring because the input itself is invisible.
 */
export function Scrubber({ position, duration, producedRanges, onSeek }: ScrubberProps) {
  const { t } = useTranslation(playerNamespace);
  const { value, onValueChange, onPointerDown, onKeyDown } = useScrubber(position, onSeek);
  const spoken = spokenClock(value, duration);
  return (
    <div className="group relative flex h-6 items-center rounded-sm has-[input:focus-visible]:outline-2 has-[input:focus-visible]:outline-offset-2 has-[input:focus-visible]:outline-accent">
      <ScrubberTrack value={value} duration={duration} producedRanges={producedRanges} />
      <input
        type="range"
        min={0}
        max={duration ?? 0}
        step="any"
        value={value}
        disabled={duration === undefined}
        aria-label={t("controls.position")}
        aria-valuetext={
          spoken.duration === undefined
            ? spoken.position
            : t("controls.positionValue", { position: spoken.position, duration: spoken.duration })
        }
        onChange={(event) => {
          onValueChange(Number(event.currentTarget.value));
        }}
        onPointerDown={onPointerDown}
        onKeyDown={onKeyDown}
        className="absolute inset-0 w-full cursor-pointer opacity-0 disabled:cursor-default"
      />
    </div>
  );
}

interface ScrubberTrackProps {
  readonly value: number;
  readonly duration: number | undefined;
  readonly producedRanges: readonly TimeRange[];
}

/** The drawn track: produced ranges shaded, the played part and the thumb in the accent. */
function ScrubberTrack({ value, duration, producedRanges }: ScrubberTrackProps) {
  const playedPercent = `${String(trackPercent(value, duration))}%`;
  return (
    <div
      aria-hidden="true"
      className="relative h-1.5 w-full rounded-full bg-surface-raised transition-[height] group-hover:h-2 motion-reduce:transition-none"
    >
      {producedRanges.map((range) => {
        const span = trackSpan(range, duration);
        return (
          <div
            key={range.start}
            data-testid="produced-range"
            className="absolute inset-y-0 rounded-full bg-text-muted/50"
            style={{
              left: `${String(span.startPercent)}%`,
              width: `${String(span.widthPercent)}%`,
            }}
          />
        );
      })}
      <div
        className="absolute inset-y-0 left-0 rounded-full bg-accent"
        style={{ width: playedPercent }}
      />
      <div
        className="absolute top-1/2 size-3.5 -translate-1/2 rounded-full bg-accent"
        style={{ left: playedPercent }}
      />
    </div>
  );
}
