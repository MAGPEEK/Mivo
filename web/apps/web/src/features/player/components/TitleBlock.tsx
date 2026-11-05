import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import type { PlayerItemInfo } from "../hooks/usePlayerItem";
import { displayedClock } from "../lib/playbackTime";
import { playerNamespace } from "../locales";

/** Props of {@link TitleBlock}. */
export interface TitleBlockProps {
  readonly item: PlayerItemInfo;
  /** Displayed position in seconds. */
  readonly position: number;
  /** Probed duration in seconds; `undefined` until known. */
  readonly duration: number | undefined;
}

const titleLinkClass = "rounded-sm hover:underline";

/**
 * The left block of the control bar (PL-02): the title as the page heading and a link to the
 * item (the show of an episode), `S1 · E3 · Title` for an episode, the year and the time.
 */
export function TitleBlock({ item, position, duration }: TitleBlockProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div className="min-w-0">
      <div className="flex min-w-0 items-baseline justify-center gap-2 md:justify-start">
        <h1 className="truncate font-semibold">
          {item.linkTitle === undefined ? (
            t("heading")
          ) : (
            <Link
              to="/items/$itemId"
              params={{ itemId: item.linkItemId }}
              className={titleLinkClass}
            >
              {item.linkTitle}
            </Link>
          )}
        </h1>
        {item.year === undefined ? null : (
          <span className="shrink-0 text-sm text-text-muted">
            {t("title.year", { year: item.year })}
          </span>
        )}
      </div>
      {item.episode && item.title !== undefined ? <EpisodeLine item={item} /> : null}
      <TimeText position={position} duration={duration} />
    </div>
  );
}

function EpisodeLine({ item }: { readonly item: PlayerItemInfo }) {
  const { t } = useTranslation(playerNamespace);
  return (
    <p className="truncate text-sm">
      {item.episodeCode === undefined
        ? item.title
        : t("title.episode", { code: item.episodeCode, title: item.title })}
    </p>
  );
}

function TimeText({ position, duration }: Omit<TitleBlockProps, "item">) {
  const { t } = useTranslation(playerNamespace);
  const clock = displayedClock(position, duration);
  return (
    <p className="text-xs whitespace-nowrap text-text-muted tabular-nums">
      {clock.duration === undefined
        ? clock.position
        : t("controls.time", { position: clock.position, duration: clock.duration })}
    </p>
  );
}
