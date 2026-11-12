import { useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { ConfirmDialog, IconFrame } from "../../../shared/ui";
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
 * what it was is the visible sign. Unmarking a show or season asks first: it clears every
 * resume position of its episodes.
 */
export function WatchedCardToggle({ title, toggle }: WatchedCardToggleProps) {
  const { t } = useTranslation(libraryNamespace);
  const [confirming, setConfirming] = useState(false);
  // True from opening the dialog until focus is back on the button, which the dialog's cleanup
  // does after the dialog is gone: the card's buttons must stay displayed until then, or the
  // focus call has no target.
  const [keepShown, setKeepShown] = useState(false);
  if (!toggle.available || toggle.failure === "unsupported") {
    return null;
  }
  const label = t(toggle.watched ? "watch.markUnwatchedTitled" : "watch.markWatchedTitled", {
    title,
  });
  return (
    <>
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        aria-label={label}
        title={toggle.watched ? t("watch.markUnwatched") : t("watch.markWatched")}
        aria-disabled={toggle.pending}
        onFocus={() => {
          setKeepShown(false);
        }}
        onClick={() => {
          if (toggle.confirmUnmark) {
            setKeepShown(true);
            setConfirming(true);
          } else {
            toggle.toggle();
          }
        }}
        className={buttonClass}
      >
        <ToggleIcon watched={toggle.watched} />
      </button>
      {keepShown && <span data-keep-open="" hidden />}
      {confirming && (
        <ConfirmUnmark
          title={title}
          onConfirm={() => {
            setConfirming(false);
            toggle.toggle();
          }}
          onCancel={() => {
            setConfirming(false);
          }}
        />
      )}
      <FailureStatus failed={toggle.failure === "failed"} />
    </>
  );
}

function FailureStatus({ failed }: { readonly failed: boolean }) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <span role="status" className="sr-only">
      {failed ? t("watch.failed") : ""}
    </span>
  );
}

function ToggleIcon({ watched }: { readonly watched: boolean }) {
  return (
    <IconFrame className="size-4 shrink-0">
      {watched ? <path d="M6 6l12 12M18 6L6 18" /> : <path d="M5 12.5l4.5 4.5L19 7.5" />}
    </IconFrame>
  );
}

interface ConfirmUnmarkProps {
  readonly title: string;
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
}

// A portal: the card's hover transform would otherwise anchor the fixed overlay to the card.
function ConfirmUnmark({ title, onConfirm, onCancel }: ConfirmUnmarkProps) {
  const { t } = useTranslation(libraryNamespace);
  return createPortal(
    <ConfirmDialog
      title={t("watch.confirmUnmarkTitle", { title })}
      description={t("watch.confirmUnmarkDescription")}
      confirmLabel={t("watch.confirmUnmark")}
      cancelLabel={t("watch.cancel")}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />,
    document.body,
  );
}
