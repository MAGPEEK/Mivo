import { productName } from "@mivo/ui-tokens";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { MetadataAttribution, creditedMetadataSources } from "../../../shared/ui";
import { useSystemInfo } from "../../system";
import { settingsNamespace } from "../locales";
import { SettingsPageHeading } from "./SettingsPageHeading";

function ServerVersionRow() {
  const { t } = useTranslation(settingsNamespace);
  const systemInfo = useSystemInfo();
  let version = t("about.loading");
  if (systemInfo.isSuccess) {
    version = systemInfo.data.version;
  } else if (systemInfo.isError) {
    version = t("about.unknown");
  }
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
      <dt className="text-text-muted">{t("about.serverVersion")}</dt>
      <dd>
        <code>{version}</code>
      </dd>
    </dl>
  );
}

/** Settings › About: the server version, the privacy promise and the provider credits. */
export function AboutSettingsPage() {
  const { t } = useTranslation(settingsNamespace);
  const attributionId = useId();
  return (
    <>
      <SettingsPageHeading title={t("about.heading")} />
      <div className="flex max-w-3xl flex-col gap-6">
        <ServerVersionRow />
        <p className="text-sm text-text-muted">{t("about.privacy", { productName })}</p>
        <section
          aria-labelledby={attributionId}
          className="rounded-lg border border-divider bg-surface p-4"
        >
          <h2 id={attributionId} className="mb-2 text-base font-semibold">
            {t("about.attributionHeading")}
          </h2>
          <MetadataAttribution sources={creditedMetadataSources} />
        </section>
      </div>
    </>
  );
}
