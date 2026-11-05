import type { HeroItem } from "@mivo/api-client";
import { resolutionLabel } from "@mivo/player-core";

const secondsPerMinute = 60;
const minutesPerHour = 60;

/** A hero runtime split for display: "1 h 30 min" or "45 min". */
export type HeroRuntime =
  | { readonly unit: "hoursMinutes"; readonly hours: number; readonly minutes: number }
  | { readonly unit: "minutes"; readonly minutes: number };

/** Words of the meta line that depend on the language. */
export interface HeroMetaTexts {
  readonly runtime: (runtime: HeroRuntime) => string;
  /** Joins the genres, e.g. "Animation, Comedy". */
  readonly genres: (genres: readonly string[]) => string;
}

/**
 * Runtime of a feature in whole minutes; a clip shorter than a minute counts as one, because
 * the meta line only gives an idea of the length. Undefined for an unknown or zero length.
 */
export function heroRuntime(durationSeconds: number | undefined): HeroRuntime | undefined {
  if (durationSeconds === undefined || !Number.isFinite(durationSeconds) || durationSeconds <= 0) {
    return undefined;
  }
  const totalMinutes = Math.max(1, Math.round(durationSeconds / secondsPerMinute));
  const hours = Math.floor(totalMinutes / minutesPerHour);
  return hours > 0
    ? { unit: "hoursMinutes", hours, minutes: totalMinutes % minutesPerHour }
    : { unit: "minutes", minutes: totalMinutes };
}

/**
 * Facts of the meta line above the hero title (HO-01): year, runtime, genres and resolution,
 * each only when known, e.g. `["2008", "1 h 30 min", "Animation, Comedy", "1080p"]`.
 */
export function heroMetaParts(item: HeroItem, texts: HeroMetaTexts): string[] {
  const runtime = heroRuntime(item.media?.durationSeconds);
  const height = item.media?.height;
  const genres = item.genres ?? [];
  return [
    item.year === undefined ? "" : String(item.year),
    runtime === undefined ? "" : texts.runtime(runtime),
    genres.length === 0 ? "" : texts.genres(genres),
    height === undefined ? "" : resolutionLabel(height, item.media?.width),
  ].filter((part) => part !== "");
}
