import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { ChevronDownIcon, Menu } from "../../../shared/ui";
import { playChoiceState, queueHandoffState } from "../../player";
import { usePlayFromStart } from "../hooks/usePlayFromStart";
import type { PlayView, ResumeView } from "../lib/itemView";
import { itemNamespace } from "../locales";

/** Props of {@link PlayAction}. */
export interface PlayActionProps {
  readonly play: PlayView;
}

type PlayableView = Extract<PlayView, { kind: "playable" }>;

const playButtonClass = "inline-flex items-center gap-2 px-6 py-3 text-lg font-semibold";
const accentClass = "bg-accent text-on-accent hover:bg-accent-hover";
// The menu half of the split button; the line separates it from the Resume half.
const splitMenuClass = `inline-flex items-center rounded-r-md border-l border-on-accent/25 px-3 ${accentClass}`;

/**
 * The large Play button. It links to the player for the primary file; when that file cannot
 * be played it is disabled and the reason is shown next to it and read as its description.
 * The disabled button uses `aria-disabled` instead of `disabled`: it stays focusable, so
 * keyboard and screen reader users reach it and hear why it does nothing.
 *
 * An item in progress gets a split button instead (DT-05): "Resume from 42:10" and a menu
 * with "Play from start".
 */
export function PlayAction({ play }: PlayActionProps) {
  const { t } = useTranslation(itemNamespace);
  const explanationId = useId();
  if (play.kind === "none") {
    return null;
  }
  if (play.kind === "playable") {
    return play.resume === undefined ? (
      <Link
        to="/items/$itemId/play"
        params={{ itemId: play.itemId }}
        search={{ fileId: play.fileId }}
        {...handoff(play)}
        className={`${playButtonClass} self-start rounded-md ${accentClass}`}
      >
        <PlayIcon />
        {t("play.label")}
      </Link>
    ) : (
      <ResumeAction play={play} resume={play.resume} />
    );
  }
  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        aria-disabled="true"
        aria-describedby={explanationId}
        className={`${playButtonClass} cursor-not-allowed self-start rounded-md bg-surface-raised text-text-muted`}
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

// What the player route receives besides the URL: the queue of an album, or the tracks the
// viewer picked. Neither belongs in the URL (length, and what was watched stays private).
function handoff(play: PlayableView): { state?: Record<string, unknown> } {
  if (play.queue !== undefined) {
    return { state: queueHandoffState({ ids: play.queue, shuffle: false }) };
  }
  const state = play.choice === undefined ? undefined : playChoiceState(play.itemId, play.choice);
  return state === undefined ? {} : { state };
}

interface ResumeActionProps {
  readonly play: PlayableView;
  readonly resume: ResumeView;
}

function ResumeAction({ play, resume }: ResumeActionProps) {
  const { t } = useTranslation(itemNamespace);
  const playFromStart = usePlayFromStart(play);
  return (
    <div className="inline-flex self-start">
      <Link
        to="/items/$itemId/play"
        params={{ itemId: play.itemId }}
        search={{ fileId: play.fileId, t: resume.seconds }}
        {...handoff(play)}
        className={`${playButtonClass} rounded-l-md ${accentClass}`}
      >
        <PlayIcon />
        {t("resume.label", { position: resume.position })}
      </Link>
      <Menu
        trigger={<ChevronDownIcon />}
        ariaLabel={t("resume.more")}
        triggerClassName={splitMenuClass}
        align="end"
        entries={[{ id: "fromStart", label: t("resume.fromStart"), onSelect: playFromStart }]}
      />
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
