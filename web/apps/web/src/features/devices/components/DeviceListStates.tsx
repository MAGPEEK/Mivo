import { useTranslation } from "react-i18next";

import { EmptyState, secondaryButtonClass } from "../../../shared/ui";
import type { DevicesFailure } from "../lib/devices";
import { devicesNamespace } from "../locales";

/** Shown while the device list loads. */
export function DevicesLoading() {
  const { t } = useTranslation(devicesNamespace);
  return (
    <p role="status" className="text-sm text-text-muted">
      {t("state.loading")}
    </p>
  );
}

/** Props of {@link DevicesFailureState}. */
export interface DevicesFailureStateProps {
  readonly failure: DevicesFailure;
  readonly onRetry: () => void;
}

/**
 * Why the list is missing. A server without accounts (501) and a refused admin list (403) are
 * lasting answers, so only network and server failures offer "Try again".
 */
export function DevicesFailureState({ failure, onRetry }: DevicesFailureStateProps) {
  const { t } = useTranslation(devicesNamespace);
  if (failure === "notAvailable") {
    return (
      <EmptyState
        title={t("state.notAvailable.title")}
        description={t("state.notAvailable.hint")}
      />
    );
  }
  if (failure === "forbidden") {
    return (
      <EmptyState title={t("state.forbidden.title")} description={t("state.forbidden.hint")} />
    );
  }
  return (
    <div role="alert" className="flex flex-col items-start gap-3">
      <p className="text-sm text-text">
        {failure === "network" ? t("state.network") : t("state.generic")}
      </p>
      <button type="button" onClick={onRetry} className={secondaryButtonClass}>
        {t("state.retry")}
      </button>
    </div>
  );
}
