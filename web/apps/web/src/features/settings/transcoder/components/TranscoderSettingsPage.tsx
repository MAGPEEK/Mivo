import { useTranslation } from "react-i18next";

import { SettingsPageHeading } from "../../components/SettingsPageHeading";
import { settingsNamespace } from "../../locales";
import { SessionsPanel } from "./SessionsPanel";
import { TranscoderStatusPanel } from "./TranscoderStatusPanel";

/**
 * Settings › Transcoder: whether hardware acceleration works, the test button and the
 * running sessions. Read-only apart from test and stop; the acceleration setting stays in the
 * server configuration until the admin pages of milestone 7.
 */
export function TranscoderSettingsPage() {
  const { t } = useTranslation(settingsNamespace);
  return (
    <>
      <SettingsPageHeading title={t("transcoder.heading")} />
      <div className="flex max-w-4xl flex-col gap-6">
        <p className="text-sm text-text-muted">{t("transcoder.intro")}</p>
        <TranscoderStatusPanel />
        <SessionsPanel />
      </div>
    </>
  );
}
