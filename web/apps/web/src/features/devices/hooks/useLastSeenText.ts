import { useTranslation } from "react-i18next";

import { formatLastSeen, lastSeen } from "../lib/devices";
import { devicesNamespace } from "../locales";

/** Returns a formatter for "last seen" times in the UI language, relative to now. */
export function useLastSeenText(): (lastSeenAt: string) => string {
  const { t, i18n } = useTranslation(devicesNamespace);
  return (lastSeenAt) =>
    formatLastSeen(lastSeen(lastSeenAt, new Date()), i18n.language, t("table.justNow"));
}
