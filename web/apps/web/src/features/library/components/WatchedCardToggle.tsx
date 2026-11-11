import { useTranslation } from "react-i18next";

import { IconFrame } from "../../../shared/ui";
import type { WatchedToggle } from "../hooks/useWatchedToggle";
import { libraryNamespace } from "../locales";

/** Props of {@link WatchedCardToggle}. */
export interface WatchedCardToggleProps {
  /** Title of the card's item; part of the button's name, so every card's button differs. */
  readonly title: string;
  readonly toggle: WatchedToggle;
}

const buttonClass =
  "grid size-8 place-items-center rounded-full bg-scrim/75 text-text hover:bg-accent hover:text-on-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent aria-disabled:cursor-not-allowed aria-disabled:text-text-muted";

/**
 * The watched toggle a card shows on hover or focus (LB-41). Renders nothing for kinds without
 * the action and once the server said it cannot store watch state: a card has no room for the
 * reason, the detail page gives it. A failed request is announced; the badge going back to
 * what it was is the visible sign.
 */
export function WatchedCardToggle({ title, toggle }: WatchedCardToggleProps) {
  const { t } = useTranslation(libraryNamespace);
  if (!toggle.available || toggle.failure === "unsupported") {
    return null;
  }
  const label = toggle.watched
    ? t("watch.markUnwatchedTitled", { title })
    : t("watch.markWatchedTitled", { title });
  return (
    <>
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        aria-label={label}
        title={toggle.watched ? t("watch.markUnwatched") : t("watch.markWatched")}
        aria-disabled={toggle.pending}
        onClick={toggle.toggle}
        className={buttonClass}
      >
        <IconFrame className="size-4 shrink-0">
          {toggle.watched ? <path d="M6 6l12 12M18 6L6 18" /> : <path d="M5 12.5l4.5 4.5L19 7.5" />}
        </IconFrame>
      </button>
      <span role="status" className="sr-only">
        {toggle.failure === "failed" ? t("watch.failed") : ""}
      </span>
    </>
  );
}
