import type { HeroItem } from "@mivo/api-client";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { heroMetaParts, type HeroMetaTexts } from "../lib/heroMeta";
import { homeNamespace } from "../locales";

/** Returns the facts of a hero title's meta line in the UI language. */
export function useHeroMeta(): (item: HeroItem) => string[] {
  const { t, i18n } = useTranslation(homeNamespace);
  const language = i18n.language;
  return useMemo(() => {
    const genreList = new Intl.ListFormat(language, { style: "short", type: "unit" });
    const texts: HeroMetaTexts = {
      runtime: (runtime) =>
        runtime.unit === "hoursMinutes"
          ? t("hero.runtime.hoursMinutes", { hours: runtime.hours, minutes: runtime.minutes })
          : t("hero.runtime.minutes", { minutes: runtime.minutes }),
      genres: (genres) => genreList.format(genres),
    };
    return (item: HeroItem) => heroMetaParts(item, texts);
  }, [language, t]);
}
