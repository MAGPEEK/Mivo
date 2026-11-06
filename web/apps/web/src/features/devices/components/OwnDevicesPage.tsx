import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { useSession } from "../../auth";
import { adminDevicesPath } from "../lib/devicePaths";
import { devicesNamespace } from "../locales";
import { DevicesSection } from "./DevicesSection";

/** The devices signed in to the own account, each with Revoke (US-25). */
export function OwnDevicesPage() {
  const { t } = useTranslation(devicesNamespace);
  const session = useSession();
  const isAdmin = session.me?.currentProfile?.isAdmin === true;
  return (
    <div className="max-w-5xl">
      <h1 className="mb-2 text-3xl font-semibold">{t("own.heading")}</h1>
      <p className="mb-6 text-sm text-text-muted">{t("own.description")}</p>
      <DevicesSection scope="own" />
      {isAdmin && (
        <p className="mt-6 text-sm">
          <Link to={adminDevicesPath} className="text-accent underline hover:text-accent-hover">
            {t("own.adminLink")}
          </Link>
        </p>
      )}
    </div>
  );
}
