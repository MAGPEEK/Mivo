import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

/** Shown for every path that matches no route. */
export function NotFoundPage() {
  const { t } = useTranslation();
  return (
    <>
      <h1 className="text-3xl font-semibold">{t("notFound.heading")}</h1>
      <p className="mt-2 text-text-muted">{t("notFound.message")}</p>
      <Link
        to="/"
        className="mt-6 inline-block rounded-md bg-accent px-4 py-2 font-medium text-on-accent hover:bg-accent-hover"
      >
        {t("notFound.backHome")}
      </Link>
    </>
  );
}
