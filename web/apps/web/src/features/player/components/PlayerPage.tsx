import { useTranslation } from "react-i18next";

import { playerNamespace } from "../locales";

/** Props of {@link PlayerPage}, from the route `/items/$itemId/play` and its search. */
export interface PlayerPageProps {
  readonly itemId: string;
  /** File to play; `undefined` plays the item's primary file. */
  readonly fileId: string | undefined;
  /** Start position in seconds; `undefined` starts at the beginning. */
  readonly startAt: number | undefined;
}

/**
 * Full-screen player page, rendered outside the app layout. Placeholder until the player
 * engine and controls arrive (M2-27, M2-30).
 */
export function PlayerPage({ itemId, fileId, startAt }: PlayerPageProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div className="px-6 py-8">
      <h1 className="mb-6 text-3xl font-semibold">{t("heading")}</h1>
      <p className="text-text-muted">
        {t("placeholder", { itemId, fileId: fileId ?? t("primaryFile"), startAt: startAt ?? 0 })}
      </p>
    </div>
  );
}
