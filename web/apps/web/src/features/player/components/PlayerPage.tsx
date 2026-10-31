import { useTranslation } from "react-i18next";

import { usePlaybackSession } from "../hooks/usePlaybackSession";
import { playbackStatusKey } from "../lib/playbackStatusText";
import { playerNamespace } from "../locales";

/** Props of {@link PlayerPage}, from the route `/items/$itemId/play` and its search. */
export interface PlayerPageProps {
  readonly itemId: string;
  /** File to play; `undefined` plays the item's primary file. */
  readonly fileId: string | undefined;
  /** Start position in seconds; `undefined` starts at the beginning. */
  readonly startAt: number | undefined;
}

/**
 * Full-screen player page, rendered outside the app layout. A minimal shell around the player
 * engine (M2-27): the native controls and a status line stand in until the page UI with its
 * own controls and the stream-info overlay arrives (M2-30).
 */
export function PlayerPage({ itemId, fileId, startAt }: PlayerPageProps) {
  const { t } = useTranslation(playerNamespace);
  // Destructured: the React compiler treats an object holding a ref callback as a ref.
  const { attachVideo, status, waitingForStream, error, retry } = usePlaybackSession({
    itemId,
    fileId,
    startAt,
  });
  return (
    <div className="px-6 py-8">
      <h1 className="mb-6 text-3xl font-semibold">{t("heading")}</h1>
      <p className="text-text-muted">
        {t("placeholder", { itemId, fileId: fileId ?? t("primaryFile"), startAt: startAt ?? 0 })}
      </p>
      {/* The server offers no subtitle tracks before milestone 4, so there is nothing to add. */}
      {/* eslint-disable-next-line jsx-a11y/media-has-caption -- no caption tracks until M4 */}
      <video
        ref={attachVideo}
        aria-label={t("video")}
        className="mt-6 aspect-video w-full rounded-md bg-surface"
        controls
        playsInline
      />
      <p role="status" className="mt-4 text-text-muted">
        {t(playbackStatusKey({ status, waitingForStream, error }))}
      </p>
      {status === "error" ? (
        <button
          type="button"
          onClick={retry}
          className="mt-4 rounded-md bg-accent px-4 py-2 font-medium text-on-accent hover:bg-accent-hover"
        >
          {t("retry")}
        </button>
      ) : null}
    </div>
  );
}
