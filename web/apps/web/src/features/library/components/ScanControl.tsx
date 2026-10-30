import type { Library } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { useLibraryScan } from "../hooks/useLibraryScan";
import { useScanStatusText } from "../hooks/useScanStatusText";
import { libraryNamespace } from "../locales";
import { primaryButtonClass } from "./buttonClasses";

/** Props of {@link ScanControl}. */
export interface ScanControlProps {
  readonly library: Library;
}

/**
 * Scan button with its status line. The status is a polite live region, so progress, the
 * result and errors are announced without moving focus.
 */
export function ScanControl({ library }: ScanControlProps) {
  const { t } = useTranslation(libraryNamespace);
  const scan = useLibraryScan(library);
  const statusText = useScanStatusText(library.scan, scan.running, scan.startError);
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        onClick={scan.start}
        aria-disabled={scan.running}
        aria-busy={scan.running}
        className={primaryButtonClass}
      >
        {scan.running ? t("scan.running") : t("scan.start")}
      </button>
      <p role="status" className="text-sm text-text-muted">
        {statusText}
      </p>
    </div>
  );
}
