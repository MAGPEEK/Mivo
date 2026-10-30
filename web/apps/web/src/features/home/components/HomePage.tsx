import { useTranslation } from "react-i18next";

import { EmptyState, LibraryIcon } from "../../../shared/ui";

/**
 * Home page. Until libraries exist it explains how content will appear; the hero area and
 * media rows replace the empty state in milestone 5.
 */
export function HomePage() {
  const { t } = useTranslation();
  return (
    <>
      <h1 className="mb-6 text-3xl font-semibold">{t("home.heading")}</h1>
      <EmptyState
        icon={<LibraryIcon className="size-10" />}
        title={t("home.empty.title")}
        description={t("home.empty.hint")}
      />
    </>
  );
}
