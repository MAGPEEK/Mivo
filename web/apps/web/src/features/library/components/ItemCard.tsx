import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { rowCardSizes, type CardFormat } from "../../../shared/lib/rowScroll";
import {
  AlertIcon,
  Badge,
  FilmIcon,
  MediaCard,
  Poster,
  ScreenIcon,
  useFormatEpisodeCode,
  type RowCardProps,
} from "../../../shared/ui";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import {
  cardArtwork,
  cardEpisodeNumbers,
  toItemCardModel,
  type ItemCardModel,
} from "../lib/itemCard";
import { libraryNamespace } from "../locales";
import { itemPosterSizes } from "./gridLayout";

/** Props of {@link ItemCard}. */
export interface ItemCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
  /** Card shape: poster 2:3 (default), episode still 16:9 or square 1:1. */
  readonly format?: CardFormat;
  /**
   * Season of an episode, for its `S3 · E9` label; rows know it from their source while the
   * item summary does not carry it.
   */
  readonly seasonNumber?: number;
  /** Badges or progress over the artwork; filled by the watched state (M7). */
  readonly overlay?: ReactNode;
  /** Set when the card sits in a `MediaRow`: the row's tab stop and a fixed card width. */
  readonly rowCard?: RowCardProps;
}

const placeholderIconClass = "size-1/4";

/** Card of one item in a grid or row; opens the item detail page. */
export function ItemCard({
  item,
  focusOnMount = false,
  format = "poster",
  seasonNumber,
  overlay,
  rowCard,
}: ItemCardProps) {
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  const formatEpisodeCode = useFormatEpisodeCode();
  const card = toItemCardModel(item);
  const artwork = cardArtwork(item, format);
  const episodeNumbers = cardEpisodeNumbers(item, seasonNumber);
  const episodeCode = episodeNumbers === undefined ? undefined : formatEpisodeCode(episodeNumbers);
  return (
    <MediaCard
      title={card.title}
      poster={
        <Poster
          imageRef={artwork.imageRef}
          aspect={artwork.aspect}
          alt={card.title}
          sizes={rowCard === undefined ? itemPosterSizes : rowCardSizes(format)}
          icon={
            card.isShow ? (
              <ScreenIcon className={placeholderIconClass} />
            ) : (
              <FilmIcon className={placeholderIconClass} />
            )
          }
        />
      }
      overlay={overlay}
      muted={!card.available}
      headingLevel={rowCard === undefined ? 2 : 3}
      renderLink={(linkProps) => (
        <Link
          to="/items/$itemId"
          params={{ itemId: card.id }}
          ref={linkRef}
          {...rowCard}
          {...linkProps}
        />
      )}
      details={<CardDetails card={card} episodeCode={episodeCode} />}
    />
  );
}

interface CardDetailsProps {
  readonly card: ItemCardModel;
  readonly episodeCode: string | undefined;
}

function CardDetails({ card, episodeCode }: CardDetailsProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      {episodeCode !== undefined && <span>{episodeCode}</span>}
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
      {card.unmatched && <Badge>{t("items.unmatched")}</Badge>}
    </>
  );
}
