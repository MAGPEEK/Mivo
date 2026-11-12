import type { ItemSummary } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

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
}

const triggerClass =
  "grid size-8 place-items-center rounded-full bg-scrim/75 text-text hover:bg-accent hover:text-on-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * Card of a Continue watching row: a click plays from the resume position, the overflow menu
 * removes the entry or marks the item watched (HO-03, LB-01). A failed action is announced.
 */
export function ContinueWatchingCard({ item, format, rowCard }: ContinueWatchingCardProps) {
  const { t } = useTranslation(libraryNamespace);
  const actions = useContinueWatchingActions(item.id);
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
          triggerClassName={triggerClass}
          trigger={<MoreIcon className="size-4" />}
          entries={[
            {
              id: "remove",
              label: t("watch.removeFromContinue"),
              disabled: actions.pending,
              onSelect: actions.remove,
            },
            {
              id: "markWatched",
              label: t("watch.markWatched"),
              disabled: actions.pending,
              onSelect: actions.markWatched,
            },
          ]}
        />
      }
    />
  );
}
