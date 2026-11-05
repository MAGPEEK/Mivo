import { useTranslation } from "react-i18next";

import { settingsNamespace } from "../locales";
import { SettingsNav } from "./SettingsNav";

/** Props of {@link SettingsBreadcrumb} and {@link SettingsPageHeading}. */
export interface SettingsPageHeadingProps {
  /** Page title; also the last breadcrumb entry. */
  readonly title: string;
}

/** "Settings › <title>" above the title of a settings page. */
export function SettingsBreadcrumb({ title }: SettingsPageHeadingProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <nav aria-label={t("libraries.breadcrumb.label")} className="mb-2 text-sm text-text-muted">
      <ol className="flex gap-2">
        <li>{t("libraries.breadcrumb.settings")}</li>
        <li aria-hidden="true">›</li>
        <li aria-current="page">{title}</li>
      </ol>
    </nav>
  );
}

/** Breadcrumb, title and page navigation of a settings page without own actions. */
export function SettingsPageHeading({ title }: SettingsPageHeadingProps) {
  return (
    <>
      <SettingsBreadcrumb title={title} />
      <h1 className="mb-4 text-3xl font-semibold">{title}</h1>
      <SettingsNav />
    </>
  );
}
