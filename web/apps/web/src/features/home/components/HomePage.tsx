import { useTranslation } from "react-i18next";

/** Placeholder home page until the app shell and media rows arrive (M1-15, milestone 5). */
export function HomePage() {
  const { t } = useTranslation();
  return (
    <>
      <h1 className="text-3xl font-semibold">{t("home.heading")}</h1>
      <p className="mt-2 text-text-muted">{t("home.placeholder")}</p>
    </>
  );
}
