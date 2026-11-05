import { useId } from "react";
import { useTranslation } from "react-i18next";

import {
  MetadataAttribution,
  Skeleton,
  creditedMetadataSources,
  secondaryButtonClass,
} from "../../../../shared/ui";
import { SettingsPageHeading } from "../../components/SettingsPageHeading";
import { settingsNamespace } from "../../locales";
import { useMetadataSettings, type MetadataSettingsControls } from "../hooks/useMetadataSettings";
import { MetadataSettingsForm } from "./MetadataSettingsForm";
import { MetadataStatusPanel } from "./MetadataStatusPanel";
import { PrivacyNote } from "./PrivacyNote";

function SettingsContent({ controls }: { readonly controls: MetadataSettingsControls }) {
  const { t } = useTranslation(settingsNamespace);
  const { load } = controls;
  switch (load.status) {
    case "loading":
      return (
        <div role="status" className="w-60">
          <Skeleton label={t("metadata.loading")} className="h-5 w-full" />
        </div>
      );
    case "unavailable":
      return (
        <p className="rounded-lg border border-divider bg-surface p-4 text-sm">
          {t("metadata.unavailable")}
        </p>
      );
    case "error":
      return (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p>{t("metadata.loadFailed")}</p>
          <button type="button" onClick={load.retry} className={secondaryButtonClass}>
            {t("metadata.retry")}
          </button>
        </div>
      );
    case "ready":
      return <MetadataSettingsForm settings={load.settings} controls={controls} />;
  }
}

function AttributionSection() {
  const { t } = useTranslation(settingsNamespace);
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border border-divider bg-surface p-4"
    >
      <h2 id={headingId} className="mb-2 text-base font-semibold">
        {t("metadata.attributionHeading")}
      </h2>
      <MetadataAttribution sources={creditedMetadataSources} />
    </section>
  );
}

/**
 * Settings › Metadata: what goes online, the switches and TMDB key, the provider status and
 * the credits the providers require. A server without the feature (501) still shows the
 * privacy note and the credits.
 */
export function MetadataSettingsPage() {
  const { t } = useTranslation(settingsNamespace);
  const controls = useMetadataSettings();
  return (
    <>
      <SettingsPageHeading title={t("metadata.heading")} />
      <div className="flex max-w-3xl flex-col gap-6">
        <p className="text-sm text-text-muted">{t("metadata.intro")}</p>
        <PrivacyNote />
        <SettingsContent controls={controls} />
        <MetadataStatusPanel />
        <AttributionSection />
      </div>
    </>
  );
}
