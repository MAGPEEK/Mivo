import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { EmptyState, LibraryIcon, primaryButtonClass } from "../../../shared/ui";
import { useLibraries } from "../../library";

/**
 * Home page. Without libraries it leads to Settings › Libraries (HO-09); the hero area and
 * media rows replace the other empty state in milestone 5.
 */
export function HomePage() {
  const { t } = useTranslation();
  const libraries = useLibraries();
  const noLibraries = libraries.isSuccess && libraries.data.length === 0;
  return (
    <>
      <h1 className="mb-6 text-3xl font-semibold">{t("home.heading")}</h1>
      {noLibraries && (
        <EmptyState
          icon={<LibraryIcon className="size-10" />}
          title={t("home.noLibraries.title")}
          description={t("home.noLibraries.hint")}
          action={
            <Link
              to="/admin/manage/libraries"
              search={{ add: true }}
              className={`inline-block ${primaryButtonClass}`}
            >
              {t("home.noLibraries.add")}
            </Link>
          }
        />
      )}
      {libraries.isSuccess && !noLibraries && (
        <EmptyState
          icon={<LibraryIcon className="size-10" />}
          title={t("home.empty.title")}
          description={t("home.empty.hint")}
        />
      )}
    </>
  );
}
