import type { HeroItem } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { InfoIcon, PlayIcon, primaryButtonClass, secondaryButtonClass } from "../../../shared/ui";
import { useHeroMeta } from "../hooks/useHeroMeta";
import { useHeroPlay } from "../hooks/useHeroPlay";
import { heroTextColumnClass } from "../lib/heroScrim";
import { homeNamespace } from "../locales";

/** Props of {@link HeroSlide}. */
export interface HeroSlideProps {
  readonly item: HeroItem;
  /** 1-based place of the title in the hero. */
  readonly position: number;
  readonly count: number;
}

const actionClass = "inline-flex items-center gap-2";
const iconClass = "size-4 shrink-0";

/** The text and actions of one hero title. Logos arrive with M6; the title shows as text. */
export function HeroSlide({ item, position, count }: HeroSlideProps) {
  const { t } = useTranslation(homeNamespace);
  const meta = useHeroMeta()(item);
  return (
    <div
      role="group"
      aria-roledescription={t("hero.slideRoleDescription")}
      aria-label={t("hero.slide", { position, count })}
      className={heroTextColumnClass}
    >
      {meta.length > 0 && <p className="mb-2 text-sm text-text-muted">{meta.join(" · ")}</p>}
      <h2 className="mb-2 text-3xl font-semibold tracking-tight sm:text-4xl md:text-5xl">
        {item.title}
      </h2>
      {item.summaryLine !== undefined && (
        <p className="mb-4 line-clamp-2 max-sm:hidden">{item.summaryLine}</p>
      )}
      <div className="mt-4 flex flex-wrap gap-2">
        <HeroPlayAction item={item} />
        <Link
          to="/items/$itemId"
          params={{ itemId: item.itemId }}
          className={`${actionClass} ${secondaryButtonClass}`}
        >
          <InfoIcon className={iconClass} />
          {t("hero.moreInfo")}
        </Link>
      </div>
    </div>
  );
}

function HeroPlayAction({ item }: { readonly item: HeroItem }) {
  const { t } = useTranslation(homeNamespace);
  const play = useHeroPlay(item);
  const content = (
    <>
      <PlayIcon className={`${iconClass} fill-current`} />
      {t("hero.play")}
    </>
  );
  if (play.kind === "link") {
    return (
      <Link
        to="/items/$itemId/play"
        params={{ itemId: play.itemId }}
        search={{}}
        className={`${actionClass} ${primaryButtonClass}`}
      >
        {content}
      </Link>
    );
  }
  return (
    <button
      type="button"
      aria-disabled={play.busy ? "true" : undefined}
      onClick={play.start}
      className={`${actionClass} ${primaryButtonClass}`}
    >
      {content}
    </button>
  );
}
