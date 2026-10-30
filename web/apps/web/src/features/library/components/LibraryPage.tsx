import { useTranslation } from "react-i18next";

import { libraryNamespace } from "../locales";

/** Props of {@link LibraryPage}. */
export interface LibraryPageProps {
  /** Library from the route `/libraries/$libraryId`. */
  readonly libraryId: string;
}

/** Library page. Placeholder until the poster grid and the scan button arrive (M2-22). */
export function LibraryPage({ libraryId }: LibraryPageProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <h1 className="mb-6 text-3xl font-semibold">{t("heading")}</h1>
      <p className="text-text-muted">{t("placeholder", { libraryId })}</p>
    </>
  );
}
