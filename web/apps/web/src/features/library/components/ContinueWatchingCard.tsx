import type { ItemSummary } from "@mivo/api-client";
import { useRef } from "react";
import { useTranslation } from "react-i18next";

import { announcePolitely } from "../../../shared/lib/announce";
import { focusNeighbourOf } from "../../../shared/lib/focusNeighbour";
import type { CardFormat } from "../../../shared/lib/rowScroll";
import { resumeSeconds } from "../../../shared/lib/watchState";
import { Menu, MoreIcon, type RowCardProps } from "../../../shared/ui";
import { useContinueWatchingActions } from "../hooks/useContinueWatchingActions";
import { libraryNamespace } from "../locales";
import { RowItemCard } from "./RowItemCard";

/** Props of {@link ContinueWatchingCard}. */
export interface ContinueWatchingCardProps {
  readonly item: ItemSummary;
  readonly format: CardFormat;
  readonly rowCard: RowCardProps;
  /**
   * Which edge of the menu button the popup aligns to: `end` keeps it inside the row on the
   * last cards, `start` keeps it clear of the sidebar on the first one.
   */
  readonly menuAlign: "start" | "end";
}

const announcementKeys = {
  remove: "watch.removedAnnouncement",
  markWatched: "watch.markedWatchedAnnouncement",
} as const;

const triggerClass =
  "grid size-8 place-items-center rounded-full bg-scrim/75 text-text hover:bg-accent hover:text-on-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * Card of a Continue watching row: a click plays from the resume position, the overflow menu
 * removes the entry or marks the item watched (HO-03, LB-01). A failed action is announced.
 */
export function ContinueWatchingCard({
  item,
  format,
  rowCard,
  menuAlign,
}: ContinueWatchingCardProps) {
  const { t } = useTranslation(libraryNamespace);
  // The menu button the user chose from: the card is gone once the rows are reloaded.
  const trigger = useRef<Element | null>(null);
  const actions = useContinueWatchingActions(item.id, (action) => {
    focusNeighbourOf(trigger.current);
    announcePolitely(t(announcementKeys[action]));
  });
  const choose = (run: () => void) => () => {
    trigger.current = document.activeElement;
    run();
  };
  return (
    <RowItemCard
      item={item}
      format={format}
      rowCard={rowCard}
      resumeAt={resumeSeconds(item.viewerState) ?? 0}
      status={actions.failed ? t("watch.actionFailed") : ""}
      actions={
        <Menu
          ariaLabel={t("watch.moreActions", { title: item.showTitle ?? item.title })}
          align={menuAlign}
          triggerClassName={triggerClass}
          trigger={<MoreIcon className="size-4" />}
          entries={[
            {
              id: "remove",
              label: t("watch.removeFromContinue"),
              disabled: actions.pending,
              onSelect: choose(actions.remove),
            },
            {
              id: "markWatched",
              label: t("watch.markWatched"),
              disabled: actions.pending,
              onSelect: choose(actions.markWatched),
            },
          ]}
        />
      }
    />
  );
}
