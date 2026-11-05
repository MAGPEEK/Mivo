import type { HeroItem } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { homeNamespace } from "../locales";

/** Props of {@link HeroDots}. */
export interface HeroDotsProps {
  readonly items: readonly HeroItem[];
  readonly current: number;
  readonly onSelect: (index: number) => void;
}

/**
 * One button per hero title, named after it; the current one is marked. The 24 px buttons
 * keep the small dots easy to hit (WCAG 2.5.8).
 */
export function HeroDots({ items, current, onSelect }: HeroDotsProps) {
  const { t } = useTranslation(homeNamespace);
  return (
    <div role="group" aria-label={t("hero.dots")} className="mt-4 flex gap-1">
      {items.map((item, index) => {
        const isCurrent = index === current;
        return (
          <button
            key={item.itemId}
            type="button"
            aria-label={t("hero.dot", { title: item.title })}
            aria-current={isCurrent ? "true" : undefined}
            onClick={() => {
              onSelect(index);
            }}
            className="group grid size-6 place-items-center rounded-full"
          >
            <span
              aria-hidden="true"
              className={`size-2.5 rounded-full ${isCurrent ? "bg-accent" : "bg-text-muted group-hover:bg-text"}`}
            />
          </button>
        );
      })}
    </div>
  );
}
