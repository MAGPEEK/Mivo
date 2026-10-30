import type { ScanStatus } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { errorCodeOf, errorKeyForCode } from "../lib/errorKeys";
import { libraryNamespace } from "../locales";

/**
 * The status line next to the scan button: progress while running, the counts of the last
 * finished scan, the translated reason of a failed scan or of a start request that failed.
 * Empty before the first scan. A failed start request wins over the last scan result, because
 * it is newer (`useLibraryScan` drops it once a newer scan exists).
 */
export function useScanStatusText(
  scan: ScanStatus,
  running: boolean,
  startError: Error | null,
): string {
  const { t } = useTranslation(libraryNamespace);
  if (running) {
    return t("scan.progress");
  }
  if (startError !== null) {
    return t("scan.startFailed", { reason: t(errorKeyForCode(errorCodeOf(startError))) });
  }
  if (scan.state === "failed") {
    return t("scan.failed", { reason: t(errorKeyForCode(scan.errorCode)) });
  }
  return scan.finishedAt === undefined ? "" : t("scan.finished", { ...scan.counts });
}
