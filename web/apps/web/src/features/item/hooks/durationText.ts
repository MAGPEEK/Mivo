import type { TFunction } from "i18next";

import type { DurationParts } from "../lib/duration";
import type { itemNamespace } from "../locales";

/** Translates a split duration, e.g. `1 h 30 min`. */
export function durationText(parts: DurationParts, t: TFunction<typeof itemNamespace>): string {
  switch (parts.unit) {
    case "hours":
      return t("duration.hoursMinutes", { hours: parts.hours, minutes: parts.minutes });
    case "minutes":
      return t("duration.minutes", { minutes: parts.minutes });
    case "seconds":
      return t("duration.seconds", { seconds: parts.seconds });
  }
}
