import { useTranslation } from "react-i18next";

import { itemNamespace } from "../locales";

/** Props of {@link ItemPage}. */
export interface ItemPageProps {
  /** Item from the route `/items/$itemId`. */
  readonly itemId: string;
}

/** Item detail page. Placeholder until the technical chips and the Play button arrive (M2-23). */
export function ItemPage({ itemId }: ItemPageProps) {
  const { t } = useTranslation(itemNamespace);
  return (
    <>
      <h1 className="mb-6 text-3xl font-semibold">{t("heading")}</h1>
      <p className="text-text-muted">{t("placeholder", { itemId })}</p>
    </>
  );
}
