import { useTranslation } from "react-i18next";

import { useDeviceList, type DeviceScope } from "../hooks/useDeviceList";
import { useDeviceRevoke } from "../hooks/useDeviceRevoke";
import type { DeviceRow } from "../lib/devices";
import { devicesNamespace } from "../locales";
import { DevicesFailureState, DevicesLoading } from "./DeviceListStates";
import { DevicesTable } from "./DevicesTable";
import { RevokeDeviceDialog } from "./RevokeDeviceDialog";

/** Props of {@link DevicesSection}. */
export interface DevicesSectionProps {
  readonly scope: DeviceScope;
  /** Narrows the loaded devices, e.g. by client type; all of them when omitted. */
  readonly select?: (devices: readonly DeviceRow[]) => readonly DeviceRow[];
  /** Text when devices exist but `select` leaves none. */
  readonly filteredEmptyText?: string;
}

/** The device list of `scope` with its loading and failure states and the revoke dialog. */
export function DevicesSection({ scope, select, filteredEmptyText }: DevicesSectionProps) {
  const { t } = useTranslation(devicesNamespace);
  const list = useDeviceList(scope);
  const revoke = useDeviceRevoke(scope);
  if (list.failure !== undefined) {
    return <DevicesFailureState failure={list.failure} onRetry={list.retry} />;
  }
  if (list.devices === undefined) {
    return <DevicesLoading />;
  }
  if (list.devices.length === 0) {
    return <p className="text-sm text-text-muted">{t("state.empty")}</p>;
  }
  const shown = select === undefined ? list.devices : select(list.devices);
  return (
    <>
      {shown.length === 0 ? (
        <p role="status" className="text-sm text-text-muted">
          {filteredEmptyText ?? t("state.empty")}
        </p>
      ) : (
        <DevicesTable devices={shown} showAccount={scope === "all"} onRevoke={revoke.ask} />
      )}
      <p className="mt-3 text-sm text-text-muted">{t("table.hint")}</p>
      <RevokeDeviceDialog revoke={revoke} />
    </>
  );
}
