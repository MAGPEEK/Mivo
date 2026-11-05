import { useCallback } from "react";
import { useTranslation } from "react-i18next";

import { episodeCodeMessage, type EpisodeNumbers } from "../lib/episodeCode";

/**
 * Formats the `S3 · E9` label of an episode in the current language; `undefined` while a
 * number is unknown. Detail pages and row cards share it, so the label reads alike everywhere.
 */
export function useFormatEpisodeCode(): (numbers: EpisodeNumbers) => string | undefined {
  const { t } = useTranslation();
  return useCallback(
    (numbers: EpisodeNumbers) => {
      const message = episodeCodeMessage(numbers);
      return message === undefined ? undefined : t(message.key, message.values);
    },
    [t],
  );
}
