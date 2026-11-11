import { useId } from "react";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import type { WatchedToggle } from "../../library";
import { itemNamespace } from "../locales";

/** Props of {@link WatchedAction}. */
export interface WatchedActionProps {
  readonly toggle: WatchedToggle;
}

/**
 * "Mark as watched" / "Mark as unwatched" next to Play (LB-41). Renders nothing for kinds
 * without the action. When the server cannot store watch state the button stays, disabled,
 * with the reason below it; `aria-disabled` keeps it focusable, so the reason is heard. A
 * failed request is announced the same way and the button can be pressed again.
 */
export function WatchedAction({ toggle }: WatchedActionProps) {
  const { t } = useTranslation(itemNamespace);
  const messageId = useId();
  if (!toggle.available) {
    return null;
  }
  const unsupported = toggle.failure === "unsupported";
  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        aria-disabled={unsupported || toggle.pending}
        aria-describedby={messageId}
        onClick={toggle.toggle}
        className={`${secondaryButtonClass} self-start`}
      >
        {toggle.watched ? t("watched.unmark") : t("watched.mark")}
      </button>
      <p id={messageId} role="status" className="text-sm text-text-muted empty:hidden">
        {failureText(toggle, t)}
      </p>
    </div>
  );
}

type ItemT = ReturnType<typeof useTranslation<typeof itemNamespace>>["t"];

function failureText(toggle: WatchedToggle, t: ItemT): string {
  switch (toggle.failure) {
    case "unsupported":
      return t("watched.unsupported");
    case "failed":
      return t("watched.failed");
    case "none":
      return "";
  }
}
