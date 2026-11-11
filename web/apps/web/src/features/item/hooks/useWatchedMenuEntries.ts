import { useTranslation } from "react-i18next";

import type { MenuEntry } from "../../../shared/ui";
import type { WatchedToggle } from "../../library";
import { itemNamespace } from "../locales";

/**
 * The overflow-menu entry "Mark as watched / unwatched" of the detail page (LB-41): none for
 * kinds without the action, disabled while a request runs or when the server cannot store
 * watch state (the button next to Play gives the reason).
 */
export function useWatchedMenuEntries(toggle: WatchedToggle): readonly MenuEntry[] {
  const { t } = useTranslation(itemNamespace);
  if (!toggle.available) {
    return [];
  }
  return [
    {
      id: "watched",
      label: toggle.watched ? t("watched.unmark") : t("watched.mark"),
      disabled: toggle.pending || toggle.failure === "unsupported",
      onSelect: toggle.toggle,
    },
  ];
}
