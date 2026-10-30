import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import type { PlayView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/** Props of {@link PlayAction}. */
export interface PlayActionProps {
  readonly itemId: string;
  readonly play: PlayView;
}

const playButtonClass =
  "inline-flex items-center gap-2 self-start rounded-md px-6 py-3 text-lg font-semibold";

/**
 * The large Play button. It links to the player for the primary file; when that file cannot
 * be played it is disabled and the reason is shown next to it and read as its description.
 * The disabled button uses `aria-disabled` instead of `disabled`: it stays focusable, so
 * keyboard and screen reader users reach it and hear why it does nothing.
 */
export function PlayAction({ itemId, play }: PlayActionProps) {
  const { t } = useTranslation(itemNamespace);
  const explanationId = useId();
  if (play.kind === "playable") {
    return (
      <Link
        to="/items/$itemId/play"
        params={{ itemId }}
        search={{ fileId: play.fileId }}
        className={`${playButtonClass} bg-accent text-on-accent hover:bg-accent-hover`}
      >
        <PlayIcon />
        {t("play.label")}
      </Link>
    );
  }
  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        aria-disabled="true"
        aria-describedby={explanationId}
        className={`${playButtonClass} cursor-not-allowed bg-surface-raised text-text-muted`}
      >
        <PlayIcon />
        {t("play.label")}
      </button>
      <p id={explanationId} className="text-sm text-text-muted">
        {play.explanation}
      </p>
    </div>
  );
}

// Decorative: the button always carries its text label.
function PlayIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      focusable="false"
      className="size-6 shrink-0"
    >
      <path d="M8 5.5v13a1 1 0 0 0 1.5.86l10.5-6.5a1 1 0 0 0 0-1.72L9.5 4.64A1 1 0 0 0 8 5.5Z" />
    </svg>
  );
}
