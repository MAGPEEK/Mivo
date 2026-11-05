import { useId } from "react";
import { useTranslation } from "react-i18next";

import { settingsNamespace } from "../../locales";

/** Names every host online metadata contacts and what it sends (ADR 0031 sections 1 and 5). */
export function PrivacyNote() {
  const { t } = useTranslation(settingsNamespace);
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border border-divider bg-surface p-4 text-sm"
    >
      <h2 id={headingId} className="mb-2 text-base font-semibold">
        {t("metadata.privacy.heading")}
      </h2>
      <p className="text-text-muted">{t("metadata.privacy.summary")}</p>
      <ul className="my-2 list-disc pl-5">
        <li>{t("metadata.privacy.hosts.tmdb")}</li>
        <li>{t("metadata.privacy.hosts.musicBrainz")}</li>
        <li>{t("metadata.privacy.hosts.coverArt")}</li>
      </ul>
      <p className="text-text-muted">{t("metadata.privacy.sent")}</p>
    </section>
  );
}
