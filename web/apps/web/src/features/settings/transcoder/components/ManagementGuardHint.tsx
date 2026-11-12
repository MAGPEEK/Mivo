import { useTranslation } from "react-i18next";

import { settingsNamespace } from "../../locales";

/** Explains a 403: the transcoder is managed by an admin on the local network only. */
export function ManagementGuardHint() {
  const { t } = useTranslation(settingsNamespace);
  return (
    <p role="alert" className="rounded-lg border border-divider bg-surface p-4 text-sm">
      {t("transcoder.forbidden")}
    </p>
  );
}
