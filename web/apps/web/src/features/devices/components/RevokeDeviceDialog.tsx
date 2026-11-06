import { useTranslation } from "react-i18next";

import { ConfirmDialog } from "../../../shared/ui";
import type { DeviceRevoke } from "../hooks/useDeviceRevoke";
import { devicesNamespace } from "../locales";

const failureKeys = {
  notAvailable: "revoke.failed.notAvailable",
  forbidden: "revoke.failed.forbidden",
  network: "revoke.failed.network",
  generic: "revoke.failed.generic",
} as const;

/** The "Revoke device?" confirmation; renders nothing while no device is asked about. */
export function RevokeDeviceDialog({ revoke }: { readonly revoke: DeviceRevoke }) {
  const { t } = useTranslation(devicesNamespace);
  const { target, failure } = revoke;
  if (target === undefined) {
    return null;
  }
  return (
    <ConfirmDialog
      title={t("revoke.title", { name: target.name })}
      description={target.current ? t("revoke.descriptionCurrent") : t("revoke.description")}
      confirmLabel={t("revoke.confirm")}
      cancelLabel={t("revoke.cancel")}
      onConfirm={revoke.confirm}
      onCancel={revoke.cancel}
      pending={revoke.pending}
      error={failure === undefined ? undefined : t(failureKeys[failure])}
    />
  );
}
