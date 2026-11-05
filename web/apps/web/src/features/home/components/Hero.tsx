import type { HeroItem } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { PauseIcon, PlayIcon } from "../../../shared/ui";
import { useHeroRotation, type HeroRotation } from "../hooks/useHeroRotation";
import { heroScrimClass, heroTextPanelClass } from "../lib/heroScrim";
import { homeNamespace } from "../locales";
import { HeroBackdrop } from "./HeroBackdrop";
import { HeroDots } from "./HeroDots";
import { HeroSlide } from "./HeroSlide";

/** Props of {@link Hero}. */
export interface HeroProps {
  /** The titles to rotate through; the hero renders nothing without any. */
  readonly items: readonly HeroItem[];
}

/**
 * The optional hero above the Home rows (HO-01): backdrop, meta line, title, summary line,
 * Play and More info, rotating through up to five titles. A labelled carousel region whose
 * first control stops the rotation (WCAG 2.2.2); slide changes are announced only while the
 * rotation is paused, so it does not talk over the user.
 */
export function Hero({ items }: HeroProps) {
  const { t } = useTranslation(homeNamespace);
  const rotation = useHeroRotation(items.length);
  const item = items[rotation.index];
  if (item === undefined) {
    return null;
  }
  return (
    <section
      aria-label={t("hero.label")}
      aria-roledescription={t("hero.roleDescription")}
      className="relative mb-8 flex min-h-60 items-end overflow-hidden rounded-lg bg-surface sm:min-h-72 md:min-h-85"
      {...rotation.pauseHandlers}
    >
      <HeroBackdrop key={item.itemId} image={item.backdrop} />
      <div aria-hidden="true" className={`absolute inset-0 ${heroScrimClass}`} />
      {rotation.canRotate && <RotationToggle rotation={rotation} />}
      <div className={heroTextPanelClass}>
        <div aria-live={rotation.rotating ? "off" : "polite"}>
          <HeroSlide
            key={item.itemId}
            item={item}
            position={rotation.index + 1}
            count={items.length}
          />
        </div>
        {items.length > 1 && (
          <HeroDots items={items} current={rotation.index} onSelect={rotation.select} />
        )}
      </div>
    </section>
  );
}

// First in the region, so keyboard users reach it before the slide; on the scrim colour, so
// the icon keeps its contrast over any backdrop.
function RotationToggle({ rotation }: { readonly rotation: HeroRotation }) {
  const { t } = useTranslation(homeNamespace);
  const label = rotation.stopped ? t("hero.startRotation") : t("hero.stopRotation");
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={rotation.toggleStopped}
      className="absolute top-3 right-3 z-10 grid size-9 place-items-center rounded-full bg-background/90 text-text hover:bg-surface-raised"
    >
      {rotation.stopped ? (
        <PlayIcon className="size-4 fill-current" />
      ) : (
        <PauseIcon className="size-4" />
      )}
    </button>
  );
}
