import { useId } from "react";
import { useTranslation } from "react-i18next";

import { Skeleton, secondaryButtonClass } from "../../../../shared/ui";
import { settingsNamespace } from "../../locales";
import type { TranscoderDevice, TranscoderStatus } from "../api/transcoderApi";
import { useTranscoderStatus } from "../hooks/useTranscoderStatus";
import { deviceResult, isSoftwareOnly, type DeviceResult } from "../lib/transcoderView";
import { HardwareTestControl } from "./HardwareTestControl";
import { ManagementGuardHint } from "./ManagementGuardHint";

// Literal keys, so a misspelled one fails type checking; values not listed read "Unknown".
const settingKeys = {
  auto: "transcoder.status.settingValue.auto",
  off: "transcoder.status.settingValue.off",
  vaapi: "transcoder.status.settingValue.vaapi",
  qsv: "transcoder.status.settingValue.qsv",
  nvenc: "transcoder.status.settingValue.nvenc",
  amf: "transcoder.status.settingValue.amf",
} as const;

const resultKeys = {
  usable: "transcoder.devices.result.usable",
  unavailable: "transcoder.devices.result.unavailable",
  degraded: "transcoder.devices.result.degraded",
} as const;

// The dot repeats the result for sighted users; the text carries it for everyone.
const resultDotClasses: Readonly<Record<DeviceResult, string>> = {
  usable: "bg-direct-play",
  unavailable: "bg-text-muted",
  degraded: "bg-transcode",
};

function formatTime(timestamp: string, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(timestamp),
  );
}

function SettingRows({ status }: { readonly status: TranscoderStatus }) {
  const { t, i18n } = useTranslation(settingsNamespace);
  const settingKey = Object.hasOwn(settingKeys, status.hwaccel)
    ? settingKeys[status.hwaccel]
    : "transcoder.status.settingValue.unknown";
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2">
      <dt className="text-text-muted">{t("transcoder.status.setting")}</dt>
      <dd>
        {t(settingKey)}
        <span className="ml-2 text-text-muted">{t("transcoder.status.settingHint")}</span>
      </dd>
      <dt className="text-text-muted">{t("transcoder.status.throttle")}</dt>
      <dd>
        {status.throttleSupported
          ? t("transcoder.status.throttleYes")
          : t("transcoder.status.throttleNo")}
      </dd>
      <dt className="text-text-muted">{t("transcoder.status.lastTested")}</dt>
      <dd>
        {status.lastTestedAt === undefined
          ? t("transcoder.status.neverTested")
          : formatTime(status.lastTestedAt, i18n.language)}
      </dd>
    </dl>
  );
}

function DeviceRow({ device }: { readonly device: TranscoderDevice }) {
  const { t } = useTranslation(settingsNamespace);
  const result = deviceResult(device);
  return (
    <li className="flex flex-col gap-0.5 py-2">
      <div className="flex items-center justify-between gap-4">
        <span>
          <span className="font-medium">{device.api.toUpperCase()}</span>{" "}
          <code className="text-text-muted">{device.encoder}</code>
        </span>
        <span className="flex items-center gap-2 text-text-muted">
          <span aria-hidden="true" className={`size-2 rounded-full ${resultDotClasses[result]}`} />
          {t(resultKeys[result])}
        </span>
      </div>
      {/* Server text, English only and free of paths; shown as plain text. */}
      {device.message !== undefined && <p className="text-text-muted">{device.message}</p>}
    </li>
  );
}

function DeviceList({ status }: { readonly status: TranscoderStatus }) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <>
      {status.devices.length > 0 && (
        <ul
          aria-label={t("transcoder.devices.label")}
          className="mt-4 divide-y divide-divider border-t border-divider"
        >
          {status.devices.map((device) => (
            <DeviceRow key={device.encoder} device={device} />
          ))}
        </ul>
      )}
      {isSoftwareOnly(status) && (
        <p className="mt-4 text-text-muted">
          {status.devices.length === 0
            ? t("transcoder.devices.none")
            : t("transcoder.devices.noneUsable")}
        </p>
      )}
    </>
  );
}

function StatusContent() {
  const { t } = useTranslation(settingsNamespace);
  const load = useTranscoderStatus();
  switch (load.state) {
    case "loading":
      return <Skeleton label={t("transcoder.loading")} className="h-5 w-48" />;
    case "forbidden":
      return <ManagementGuardHint />;
    case "error":
      return (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p>{t("transcoder.loadFailed")}</p>
          <button type="button" onClick={load.retry} className={secondaryButtonClass}>
            {t("transcoder.retry")}
          </button>
        </div>
      );
    case "ready":
      return (
        <>
          <SettingRows status={load.status} />
          <DeviceList status={load.status} />
          <HardwareTestControl test={load.test} />
        </>
      );
  }
}

/** Acceleration setting, per-encoder results and the hardware test button. */
export function TranscoderStatusPanel() {
  const { t } = useTranslation(settingsNamespace);
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border border-divider bg-surface p-4 text-sm"
    >
      <h2 id={headingId} className="mb-3 text-base font-semibold">
        {t("transcoder.status.heading")}
      </h2>
      <StatusContent />
    </section>
  );
}
