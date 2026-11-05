import { useCallback } from "react";
import { useTranslation } from "react-i18next";

import { rowTitleMessage, type RowTitleSource } from "../lib/rowTitle";

/**
 * Translates the title of a server row (Home and library Overview) in the current language;
 * unknown keys or kinds from a newer server get a fallback title instead of a raw key.
 */
export function useRowTitle(): (row: RowTitleSource) => string {
  const { t } = useTranslation();
  return useCallback(
    (row: RowTitleSource) => {
      const message = rowTitleMessage(row);
      return "values" in message ? t(message.key, message.values) : t(message.key);
    },
    [t],
  );
}
