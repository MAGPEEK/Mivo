import { useTranslation } from "react-i18next";

import { AlertIcon, CloseIcon } from "../../../shared/ui";
import type { SessionToastKind } from "../lib/sessionToasts";
import { playerNamespace } from "../locales";
import { iconButtonClass } from "./playerStyles";

/** Props of {@link PlayerToasts}. */
export interface PlayerToastsProps {
  readonly toasts: readonly SessionToastKind[];
  readonly onDismiss: () => void;
}

/**
 * What the last switch cost the viewer (PL-10, PL-12): a refused burn-in is an alert, a quality
 * cap a status. Both fade by themselves and have a close button.
 */
export function PlayerToasts({ toasts, onDismiss }: PlayerToastsProps) {
  const { t } = useTranslation(playerNamespace);
  if (toasts.length === 0) {
    return null;
  }
  return (
    <div className="pointer-events-none absolute inset-x-0 top-20 z-30 flex flex-col items-center gap-2 px-4">
      {toasts.map((kind) => (
        <div
          key={kind}
          role={kind === "burnInUnavailable" ? "alert" : "status"}
          className="pointer-events-auto flex max-w-xl items-start gap-3 rounded-lg bg-surface-raised p-3 text-sm shadow-overlay"
        >
          <AlertIcon className="mt-0.5 size-5 shrink-0 text-transcode" />
          <p>{t(`options.toast.${kind}`)}</p>
          <button
            type="button"
            onClick={onDismiss}
            aria-label={t("options.toast.close")}
            className={iconButtonClass}
          >
            <CloseIcon />
          </button>
        </div>
      ))}
    </div>
  );
}
