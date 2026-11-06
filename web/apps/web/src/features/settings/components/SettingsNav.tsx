import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { settingsNamespace } from "../locales";

// TanStack Router marks the link of the current page with aria-current="page".
const linkClass =
  "block rounded-md px-3 py-1.5 text-sm font-medium text-text-muted hover:bg-surface-raised hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text";

/**
 * Links between the settings pages. A stand-in for the admin sidebar of milestone 7, so every
 * settings page is reachable from the one the sidebar opens.
 */
export function SettingsNav() {
  const { t } = useTranslation(settingsNamespace);
  return (
    <nav aria-label={t("nav.label")} className="mb-6 border-b border-divider pb-3">
      <ul className="flex flex-wrap gap-2">
        <li>
          <Link to="/admin/manage/libraries" className={linkClass}>
            {t("nav.libraries")}
          </Link>
        </li>
        <li>
          <Link to="/admin/settings/metadata" className={linkClass}>
            {t("nav.metadata")}
          </Link>
        </li>
        <li>
          <Link to="/admin/settings/about" className={linkClass}>
            {t("nav.about")}
          </Link>
        </li>
        <li>
          <Link to="/admin/devices" className={linkClass}>
            {t("nav.devices")}
          </Link>
        </li>
      </ul>
    </nav>
  );
}
