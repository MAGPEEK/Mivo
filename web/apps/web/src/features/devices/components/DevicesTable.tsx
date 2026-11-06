import type { AddressClass, ClientKind } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { Badge } from "../../../shared/ui";
import { useLastSeenText } from "../hooks/useLastSeenText";
import type { DeviceRow } from "../lib/devices";
import { devicesNamespace } from "../locales";

/** Props of {@link DevicesTable}. */
export interface DevicesTableProps {
  readonly devices: readonly DeviceRow[];
  /** Adds the account column (Admin › Devices). */
  readonly showAccount: boolean;
  readonly onRevoke: (device: DeviceRow) => void;
}

const clientKeys = { web: "client.web", tv: "client.tv", cli: "client.cli" } as const;
const networkKeys = { lan: "network.lan", remote: "network.remote" } as const;

const headerClass = "px-3 py-2 text-left text-xs font-semibold uppercase text-text-muted";
const cellClass = "px-3 py-2 align-middle";
const revokeButtonClass =
  "rounded-md border border-divider px-3 py-1 text-sm font-medium text-accent hover:bg-surface-raised";

function ClientCell({ client }: { readonly client: ClientKind }) {
  const { t } = useTranslation(devicesNamespace);
  // Newer servers may send client kinds this version does not know; show them as sent.
  const key = (clientKeys as Partial<Record<string, (typeof clientKeys)[ClientKind]>>)[client];
  return <td className={cellClass}>{key === undefined ? client : t(key)}</td>;
}

function NetworkCell({ addressClass }: { readonly addressClass: AddressClass }) {
  const { t } = useTranslation(devicesNamespace);
  const key = (networkKeys as Partial<Record<string, (typeof networkKeys)[AddressClass]>>)[
    addressClass
  ];
  const tone = addressClass === "lan" ? "text-direct-play" : "text-transcode";
  return (
    <td className={cellClass}>
      <span
        className={`rounded-sm border border-divider px-1.5 py-0.5 text-xs font-medium ${tone}`}
      >
        {key === undefined ? addressClass : t(key)}
      </span>
    </td>
  );
}

function DeviceTableRow({
  device,
  showAccount,
  onRevoke,
}: Omit<DevicesTableProps, "devices"> & { readonly device: DeviceRow }) {
  const { t } = useTranslation(devicesNamespace);
  const lastSeenText = useLastSeenText();
  return (
    <tr className="border-t border-divider">
      <th scope="row" className={`${cellClass} text-left font-semibold`}>
        <span className="mr-2">{device.name}</span>
        {device.current && <Badge>{t("table.thisDevice")}</Badge>}
      </th>
      {showAccount && <td className={cellClass}>{device.userName}</td>}
      <ClientCell client={device.client} />
      <td className={cellClass}>{device.platform ?? t("table.unknownPlatform")}</td>
      <td className={cellClass}>
        <time dateTime={device.lastSeenAt}>{lastSeenText(device.lastSeenAt)}</time>
      </td>
      <NetworkCell addressClass={device.addressClass} />
      <td className={`${cellClass} text-right`}>
        <button
          type="button"
          aria-label={t("table.revokeNamed", { name: device.name })}
          onClick={() => {
            onRevoke(device);
          }}
          className={revokeButtonClass}
        >
          {t("table.revoke")}
        </button>
      </td>
    </tr>
  );
}

/**
 * The signed-in devices with name, client, platform, last seen, network and a Revoke button.
 * The app version column of the TV app joins once that client exists.
 */
export function DevicesTable({ devices, showAccount, onRevoke }: DevicesTableProps) {
  const { t } = useTranslation(devicesNamespace);
  return (
    <div className="overflow-x-auto rounded-lg border border-divider bg-surface">
      <table className="w-full text-sm">
        <caption className="sr-only">{t("table.label")}</caption>
        <thead>
          <tr>
            <th scope="col" className={headerClass}>
              {t("table.name")}
            </th>
            {showAccount && (
              <th scope="col" className={headerClass}>
                {t("table.account")}
              </th>
            )}
            <th scope="col" className={headerClass}>
              {t("table.client")}
            </th>
            <th scope="col" className={headerClass}>
              {t("table.platform")}
            </th>
            <th scope="col" className={headerClass}>
              {t("table.lastSeen")}
            </th>
            <th scope="col" className={headerClass}>
              {t("table.network")}
            </th>
            <th scope="col" className={headerClass}>
              <span className="sr-only">{t("table.actions")}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {devices.map((device) => (
            <DeviceTableRow
              key={device.id}
              device={device}
              showAccount={showAccount}
              onRevoke={onRevoke}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
