import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AlertIcon, Badge, FilmIcon, MediaCard, Poster, ScreenIcon } from "../../../shared/ui";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import { toItemCardModel, type ItemCardModel } from "../lib/itemCard";
import { libraryNamespace } from "../locales";
import { itemPosterSizes } from "./gridLayout";

/** Props of {@link ItemCard}. */
export interface ItemCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
}

const placeholderIconClass = "size-1/4";

/** Poster card of one item; opens the item detail page. */
export function ItemCard({ item, focusOnMount = false }: ItemCardProps) {
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  const card = toItemCardModel(item);
  return (
    <MediaCard
      title={card.title}
      poster={
        <Poster
          imageRef={card.poster}
          aspect="poster"
          alt={card.title}
          sizes={itemPosterSizes}
          icon={
            card.isShow ? (
              <ScreenIcon className={placeholderIconClass} />
            ) : (
              <FilmIcon className={placeholderIconClass} />
            )
          }
        />
      }
      muted={!card.available}
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: card.id }} ref={linkRef} {...linkProps} />
      )}
      details={<CardDetails card={card} />}
    />
  );
}

function CardDetails({ card }: { readonly card: ItemCardModel }) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      {card.year !== "" && <span>{card.year}</span>}
      {card.isShow ? (
        <span>{t("items.seasons", { count: card.seasonCount })}</span>
      ) : (
        <>
          {card.duration !== "" && <span>{card.duration}</span>}
          {card.chip !== "" && <Badge>{card.chip}</Badge>}
          {!card.available && (
            <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
              {t("items.unavailable")}
            </Badge>
          )}
        </>
      )}
    </>
  );
}
