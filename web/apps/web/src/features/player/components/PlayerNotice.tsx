import { useTranslation } from "react-i18next";

import { AlertIcon } from "../../../shared/ui";
import type { PlaybackNotice } from "../lib/playbackStatusText";
import { playerNamespace } from "../locales";
import { primaryButtonClass, secondaryButtonClass } from "./playerStyles";

/** Props of {@link PlayerNotice}. */
export interface PlayerNoticeProps {
  readonly notice: PlaybackNotice;
  readonly onRetry: () => void;
  /** Opens the item page, for errors a retry cannot fix. */
  readonly onOpenItem: () => void;
}

/** Ref callback that focuses an element when it appears, e.g. the action of a new error. */
function focusWhenShown(element: HTMLElement | null): void {
  element?.focus();
}

/**
 * The centre of the player while the video cannot play: a spinner with a status text, or an
 * error with what the user can do about it. The status region stays mounted, so screen
 * readers announce each change.
 */
export function PlayerNotice({ notice, onRetry, onOpenItem }: PlayerNoticeProps) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center p-6">
      <div role="status">
        {notice.kind === "loading" ? (
          <div className="flex flex-col items-center gap-3 rounded-lg bg-background/70 px-6 py-4">
            <span
              aria-hidden="true"
              className="size-10 animate-spin rounded-full border-4 border-surface-raised border-t-accent motion-reduce:animate-none"
            />
            <p>{t(notice.key)}</p>
          </div>
        ) : null}
      </div>
      {notice.kind === "error" ? (
        <div className="pointer-events-auto flex max-w-md flex-col items-center gap-4 rounded-lg bg-surface/95 p-6 text-center shadow-overlay">
          <AlertIcon className="size-8 text-accent" />
          <p role="alert">{t(notice.key)}</p>
          {notice.retryable ? (
            <button
              type="button"
              ref={focusWhenShown}
              onClick={onRetry}
              className={primaryButtonClass}
            >
              {t("retry")}
            </button>
          ) : (
            <button
              type="button"
              ref={focusWhenShown}
              onClick={onOpenItem}
              className={secondaryButtonClass}
            >
              {t("backToItem")}
            </button>
          )}
        </div>
      ) : null}
    </div>
  );
}
