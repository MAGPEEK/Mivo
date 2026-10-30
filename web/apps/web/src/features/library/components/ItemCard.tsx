import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AlertIcon, Badge, MediaCard } from "../../../shared/ui";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import { toItemCardModel } from "../lib/itemCard";
import { libraryNamespace } from "../locales";

/** Props of {@link ItemCard}. */
export interface ItemCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

/** Poster card of one item; opens the item detail page. */
export function ItemCard({ item, focusOnMount = false }: ItemCardProps) {
  const { t } = useTranslation(libraryNamespace);
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  const card = toItemCardModel(item);
  return (
    <MediaCard
      title={card.title}
      posterText={card.initial}
      muted={!card.available}
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: card.id }} ref={linkRef} {...linkProps} />
      )}
      details={
        <>
          {card.duration !== "" && <span>{card.duration}</span>}
          {card.chip !== "" && <Badge>{card.chip}</Badge>}
          {!card.available && (
            <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
              {t("items.unavailable")}
            </Badge>
          )}
        </>
      }
    />
  );
}
