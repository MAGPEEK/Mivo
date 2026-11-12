import type { StreamInfo, StreamInfoColorToken } from "@mivo/player-core";
import { useId, type ComponentType, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ChevronRightIcon, InfoIcon, PlayIcon, ResetIcon } from "../../../shared/ui";
import { playerNamespace } from "../locales";

/** Props of {@link ModeBadge}. */
export interface ModeBadgeProps {
  readonly info: StreamInfo;
  /** Whether the stream-info overlay is open. */
  readonly open: boolean;
  readonly onToggle: () => void;
  /** The button; focus returns here when the overlay closes. */
  readonly buttonRef: RefObject<HTMLButtonElement | null>;
}

// Status colours of ADR 0011 by player-core's token name (text and border, so a coloured
// outline and label read on any frame of the video).
const badgeColorClasses: Readonly<Record<StreamInfoColorToken, string>> = {
  directPlay: "text-direct-play border-direct-play",
  directStream: "text-direct-stream border-direct-stream",
  transcode: "text-transcode border-transcode",
  textMuted: "text-text-muted border-text-muted",
};

// A second signal besides colour: each mode has its own icon and its own name.
const badgeIcons: Readonly<Record<StreamInfoColorToken, ComponentType<{ className?: string }>>> = {
  directPlay: PlayIcon,
  directStream: ChevronRightIcon,
  transcode: ResetIcon,
  textMuted: InfoIcon,
};

/**
 * The delivery mode in the control bar (PL-40): icon and short name in the status colour. It
 * opens the stream-info overlay like the `I` key; hovering or focusing it shows why the mode
 * was chosen. It renders from the session, so a mode switch updates it at once.
 */
export function ModeBadge({ info, open, onToggle, buttonRef }: ModeBadgeProps) {
  const { t } = useTranslation(playerNamespace);
  const tooltipId = useId();
  const Icon = badgeIcons[info.mode.colorToken];
  const mode = t(info.mode.key);
  const reasons = info.reasons.map((reason) => t(reason.key, { replace: reason.params }));
  return (
    <span className="group relative inline-flex">
      <button
        ref={buttonRef}
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-describedby={tooltipId}
        aria-label={t("overlay.badge.label", { mode })}
        className={`inline-flex h-8 items-center gap-1.5 rounded-full border bg-background/60 px-3 text-xs font-bold whitespace-nowrap hover:bg-surface-raised ${badgeColorClasses[info.mode.colorToken]}`}
      >
        <Icon className="size-4" />
        {mode}
      </button>
      <span
        id={tooltipId}
        role="tooltip"
        className="pointer-events-none absolute right-0 bottom-full z-10 mb-2 hidden w-72 max-w-[80vw] rounded-md bg-surface-raised p-2 text-start text-xs font-normal text-text shadow-overlay group-focus-within:block group-hover:block"
      >
        {reasons.length === 0 ? t("overlay.badge.allCopied") : reasons.join(" · ")}
      </span>
    </span>
  );
}
