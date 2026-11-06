import { useTranslation } from "react-i18next";

import { AuthCard } from "./AuthCard";
import { SignOutButton } from "./SignOutButton";

/**
 * Stand-in for the profile picker (US-01) until M7-31 builds it: an account with managed
 * profiles or a PIN can only sign out again here.
 */
export function ProfilesPlaceholderPage() {
  const { t } = useTranslation("auth");
  return (
    <AuthCard title={t("profiles.title")}>
      <p className="mt-2 mb-4 text-sm text-text-muted">{t("profiles.placeholder")}</p>
      <SignOutButton />
    </AuthCard>
  );
}
