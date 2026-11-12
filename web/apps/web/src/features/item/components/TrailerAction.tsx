import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { itemNamespace } from "../locales";

/** Props of {@link TrailerAction}. */
export interface TrailerActionProps {
  /** The first trailer extra; the button is hidden without one. */
  readonly trailerId: string | undefined;
}

/** The trailer button next to Play: starts the first trailer through the normal player route. */
export function TrailerAction({ trailerId }: TrailerActionProps) {
  const { t } = useTranslation(itemNamespace);
  if (trailerId === undefined) {
    return null;
  }
  return (
    <Link
      to="/items/$itemId/play"
      params={{ itemId: trailerId }}
      search={{ fileId: undefined }}
      className="inline-flex items-center gap-2 self-start rounded-md border border-divider bg-surface-raised px-6 py-3 text-lg font-semibold text-text hover:bg-surface"
    >
      {t("trailer.label")}
    </Link>
  );
}
